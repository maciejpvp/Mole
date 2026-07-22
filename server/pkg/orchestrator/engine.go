package orchestrator

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	roleControl = byte(0x00)
	roleTCPLeg  = byte(0x01)
	roleUDP     = byte(0x02)

	signalNewTCP    = byte(0x01)
	signalStartUDP  = byte(0x02)
	handshakeMaxLen = 512
	dialTimeout     = 10 * time.Second
)

// Config configures one relay instance. Public ports are allocated only from
// the inclusive PortMin/PortMax range.
type Config struct {
	ControlPort int
	PortMin     int
	PortMax     int
	PublicHost  string
}

type ProvisionRequest struct {
	TunnelID                  string
	UserID                    string
	Protocol                  string
	Token                     string
	MonthlyMinutesLimit       *int64
	MonthlyTransferBytesLimit *int64
	MonthlyMinutesUsed        int64
	MonthlyTransferBytesUsed  int64
}

type ProvisionResponse struct {
	OutboundPort int    `json:"outbound_port"`
	PublicHost   string `json:"public_host"`
	ControlPort  int    `json:"control_port"`
}

type UsageUpdate struct {
	TunnelID      string `json:"tunnel_id"`
	ActiveMinutes int64  `json:"active_minutes"`
	TransferBytes int64  `json:"transfer_bytes"`
}

// ConnectionStatusUpdate is emitted when an authenticated client connects to
// or disconnects from its relay tunnel.
type ConnectionStatusUpdate struct {
	TunnelID string `json:"tunnel_id"`
	Status   string `json:"status"`
}

// Engine is the in-memory tunnel registry for one relay instance.
type Engine struct {
	cfg Config

	mu       sync.RWMutex
	tunnels  map[string]*tunnel
	tokens   map[[sha256.Size]byte]*tunnel
	users    map[string]*userUsage
	usedPort map[int]struct{}

	connectionStatusUpdates chan ConnectionStatusUpdate

	controlLn net.Listener
	stopCh    chan struct{}
	stopOnce  sync.Once
}

type userUsage struct {
	monthlyMinutesLimit       *int64
	monthlyTransferBytesLimit *int64
	minutesUsed               int64
	transferUsed              int64
	tunnels                   map[string]*tunnel
}

type tunnel struct {
	engine *Engine

	id       string
	userID   string
	protocol string
	token    [sha256.Size]byte
	port     int

	mu                     sync.Mutex
	stopped                bool
	tcpListener            net.Listener
	udpListener            *net.UDPConn
	control                net.Conn
	controlWriteMu         sync.Mutex
	tcpLegs                chan net.Conn
	sessions               map[net.Conn]struct{}
	udpBridge              net.Conn
	udpBridgeWriteMu       sync.Mutex
	activeSince            time.Time
	activeRemainderSeconds int64
	unsyncedMinutes        int64
	unsyncedTransferBytes  int64
}

func New(cfg Config) (*Engine, error) {
	if cfg.ControlPort < 1 || cfg.ControlPort > 65535 || cfg.PortMin < 1 || cfg.PortMax > 65535 || cfg.PortMin > cfg.PortMax || strings.TrimSpace(cfg.PublicHost) == "" {
		return nil, errors.New("invalid relay configuration")
	}
	return &Engine{
		cfg:                     cfg,
		tunnels:                 make(map[string]*tunnel),
		tokens:                  make(map[[sha256.Size]byte]*tunnel),
		users:                   make(map[string]*userUsage),
		usedPort:                make(map[int]struct{}),
		connectionStatusUpdates: make(chan ConnectionStatusUpdate, 256),
		stopCh:                  make(chan struct{}),
	}, nil
}

// ConnectionStatusUpdates returns relay-authenticated connection changes in
// order. The control-plane sync worker is the sole consumer.
func (e *Engine) ConnectionStatusUpdates() <-chan ConnectionStatusUpdate {
	return e.connectionStatusUpdates
}

func (e *Engine) recordConnectionStatus(tunnelID, status string) {
	select {
	case e.connectionStatusUpdates <- ConnectionStatusUpdate{TunnelID: tunnelID, Status: status}:
	default:
		log.Printf("[orchestrator] connection-status queue full; dropping %s update for %s", status, tunnelID)
	}
}

// Run starts the fixed control listener. Public listeners are created by
// Provision and removed by Deprovision.
func (e *Engine) Run() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", e.cfg.ControlPort))
	if err != nil {
		return fmt.Errorf("control listener: %w", err)
	}
	e.mu.Lock()
	e.controlLn = ln
	e.mu.Unlock()
	log.Printf("[orchestrator] control listener up on %s", ln.Addr())

	go e.acceptControl(ln)
	<-e.stopCh
	return nil
}

func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		close(e.stopCh)
		e.mu.Lock()
		if e.controlLn != nil {
			_ = e.controlLn.Close()
		}
		tunnels := make([]*tunnel, 0, len(e.tunnels))
		for _, item := range e.tunnels {
			tunnels = append(tunnels, item)
		}
		e.mu.Unlock()
		for _, item := range tunnels {
			item.closeRuntime()
		}
	})
}

func (e *Engine) Provision(request ProvisionRequest) (ProvisionResponse, error) {
	if request.TunnelID == "" || request.UserID == "" || request.Token == "" || (request.Protocol != "tcp" && request.Protocol != "udp") || request.MonthlyMinutesUsed < 0 || request.MonthlyTransferBytesUsed < 0 {
		return ProvisionResponse{}, errors.New("invalid tunnel provision request")
	}
	tokenHash := sha256.Sum256([]byte(request.Token))

	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.tunnels[request.TunnelID]; exists {
		return ProvisionResponse{}, errors.New("tunnel already exists")
	}
	if _, exists := e.tokens[tokenHash]; exists {
		return ProvisionResponse{}, errors.New("tunnel token already exists")
	}

	item := &tunnel{
		engine:   e,
		id:       request.TunnelID,
		userID:   request.UserID,
		protocol: request.Protocol,
		token:    tokenHash,
		tcpLegs:  make(chan net.Conn, 64),
		sessions: make(map[net.Conn]struct{}),
	}
	port, err := e.bindPublicListenerLocked(item)
	if err != nil {
		return ProvisionResponse{}, err
	}
	item.port = port
	e.tunnels[item.id] = item
	e.tokens[tokenHash] = item

	usage := e.users[request.UserID]
	if usage == nil {
		usage = &userUsage{
			minutesUsed:  request.MonthlyMinutesUsed,
			transferUsed: request.MonthlyTransferBytesUsed,
			tunnels:      make(map[string]*tunnel),
		}
		e.users[request.UserID] = usage
	}
	usage.monthlyMinutesLimit = copyLimit(request.MonthlyMinutesLimit)
	usage.monthlyTransferBytesLimit = copyLimit(request.MonthlyTransferBytesLimit)
	usage.tunnels[item.id] = item

	if item.protocol == "tcp" {
		go item.acceptTCP()
	} else {
		go item.relayUDP()
	}
	log.Printf("[orchestrator] provisioned %s tunnel %s on public port %d", item.protocol, item.id, item.port)
	return ProvisionResponse{OutboundPort: item.port, PublicHost: e.cfg.PublicHost, ControlPort: e.cfg.ControlPort}, nil
}

func (e *Engine) Deprovision(tunnelID string) error {
	e.mu.Lock()
	item, exists := e.tunnels[tunnelID]
	if !exists {
		e.mu.Unlock()
		return nil
	}
	delete(e.tunnels, tunnelID)
	delete(e.tokens, item.token)
	delete(e.usedPort, item.port)
	if usage := e.users[item.userID]; usage != nil {
		delete(usage.tunnels, tunnelID)
		if len(usage.tunnels) == 0 {
			delete(e.users, item.userID)
		}
	}
	e.mu.Unlock()
	item.closeRuntime()
	log.Printf("[orchestrator] deprovisioned tunnel %s", tunnelID)
	return nil
}

func (e *Engine) acceptControl(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-e.stopCh:
				return
			default:
				log.Printf("[orchestrator] control accept error: %v", err)
				continue
			}
		}
		go e.registerConnection(conn)
	}
}

func (e *Engine) registerConnection(conn net.Conn) {
	item, role, err := e.authenticate(conn)
	if err != nil {
		_ = conn.Close()
		return
	}
	if item.isStopped() {
		_ = conn.Close()
		return
	}

	switch role {
	case roleControl:
		item.setControl(conn)
		if item.protocol == "udp" && item.signal(signalStartUDP) != nil {
			item.clearControl(conn)
			_ = conn.Close()
		}
	case roleTCPLeg:
		if item.protocol != "tcp" {
			_ = conn.Close()
			return
		}
		select {
		case item.tcpLegs <- conn:
		case <-e.stopCh:
			_ = conn.Close()
		}
	case roleUDP:
		if item.protocol != "udp" {
			_ = conn.Close()
			return
		}
		item.setUDPBridge(conn)
	default:
		_ = conn.Close()
	}
}

func (e *Engine) authenticate(conn net.Conn) (*tunnel, byte, error) {
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetReadDeadline(time.Time{}) //nolint:errcheck

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, 0, err
	}
	length := int(uint32(header[0])<<24 | uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3]))
	if length < 1 || length > handshakeMaxLen {
		return nil, 0, errors.New("invalid token length")
	}
	token := make([]byte, length)
	if _, err := io.ReadFull(conn, token); err != nil {
		return nil, 0, err
	}
	role := make([]byte, 1)
	if _, err := io.ReadFull(conn, role); err != nil {
		return nil, 0, err
	}
	hash := sha256.Sum256(token)

	e.mu.RLock()
	item := e.tokens[hash]
	e.mu.RUnlock()
	if item == nil || subtle.ConstantTimeCompare(hash[:], item.token[:]) != 1 {
		return nil, 0, errors.New("invalid token")
	}
	return item, role[0], nil
}

func (e *Engine) bindPublicListenerLocked(item *tunnel) (int, error) {
	for port := e.cfg.PortMin; port <= e.cfg.PortMax; port++ {
		if _, used := e.usedPort[port]; used {
			continue
		}
		if item.protocol == "tcp" {
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				continue
			}
			item.tcpListener = ln
		} else {
			conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
			if err != nil {
				continue
			}
			item.udpListener = conn
		}
		e.usedPort[port] = struct{}{}
		return port, nil
	}
	return 0, errors.New("no public ports available")
}

func (t *tunnel) acceptTCP() {
	t.mu.Lock()
	listener := t.tcpListener
	t.mu.Unlock()
	if listener == nil {
		return
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			if !t.isStopped() {
				log.Printf("[orchestrator] tunnel %s accept error: %v", t.id, err)
			}
			return
		}
		go t.bridgeTCP(conn)
	}
}

func (t *tunnel) bridgeTCP(publicConn net.Conn) {
	defer publicConn.Close()
	if t.signal(signalNewTCP) != nil {
		return
	}
	select {
	case tunnelConn := <-t.tcpLegs:
		t.addSession(publicConn, tunnelConn)
		defer t.removeSession(publicConn, tunnelConn)
		relayTCP(publicConn, tunnelConn, func(bytes int64) { t.engine.recordBytes(t.id, bytes) })
	case <-time.After(dialTimeout):
	case <-t.engine.stopCh:
	}
}

func (t *tunnel) relayUDP() {
	t.mu.Lock()
	listener := t.udpListener
	t.mu.Unlock()
	if listener == nil {
		return
	}
	buffer := make([]byte, 65535)
	for {
		n, remote, err := listener.ReadFromUDP(buffer)
		if err != nil {
			if !t.isStopped() {
				log.Printf("[orchestrator] UDP tunnel %s read error: %v", t.id, err)
			}
			return
		}
		payload := append([]byte(nil), buffer[:n]...)
		bridge := t.getUDPBridge()
		if bridge == nil {
			continue
		}
		if err := t.writeUDPFrame(bridge, remote.String(), payload); err != nil {
			t.clearUDPBridge(bridge)
			_ = bridge.Close()
			continue
		}
		t.engine.recordBytes(t.id, int64(n))
	}
}

func (t *tunnel) setControl(conn net.Conn) {
	t.mu.Lock()
	previous := t.control
	t.control = conn
	t.activeSince = time.Now()
	t.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	t.engine.recordConnectionStatus(t.id, "active")
	go func() {
		buffer := make([]byte, 1)
		for {
			if _, err := conn.Read(buffer); err != nil {
				t.clearControl(conn)
				return
			}
		}
	}()
}

func (t *tunnel) clearControl(conn net.Conn) {
	cleared := false
	t.mu.Lock()
	minutes := int64(0)
	if t.control == conn {
		minutes = t.flushActiveDurationLocked(time.Now())
		t.control = nil
		cleared = true
		t.activeSince = time.Time{}
		if t.udpBridge != nil {
			_ = t.udpBridge.Close()
			t.udpBridge = nil
		}
	}
	t.mu.Unlock()
	t.engine.recordMinutes(t.userID, minutes)
	if cleared {
		t.engine.recordConnectionStatus(t.id, "inactive")
	}
}

func (t *tunnel) signal(signal byte) error {
	t.mu.Lock()
	conn := t.control
	stopped := t.stopped
	t.mu.Unlock()
	if stopped || conn == nil {
		return errors.New("tunnel client is not connected")
	}
	t.controlWriteMu.Lock()
	defer t.controlWriteMu.Unlock()
	_, err := conn.Write([]byte{signal})
	return err
}

func (t *tunnel) setUDPBridge(conn net.Conn) {
	t.mu.Lock()
	previous := t.udpBridge
	t.udpBridge = conn
	t.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	go t.readUDPBridge(conn)
}

func (t *tunnel) getUDPBridge() net.Conn {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.udpBridge
}

func (t *tunnel) clearUDPBridge(conn net.Conn) {
	t.mu.Lock()
	if t.udpBridge == conn {
		t.udpBridge = nil
	}
	t.mu.Unlock()
}

func (t *tunnel) readUDPBridge(conn net.Conn) {
	for {
		address, payload, err := readUDPFrame(conn)
		if err != nil {
			t.clearUDPBridge(conn)
			_ = conn.Close()
			return
		}
		remote, err := net.ResolveUDPAddr("udp", address)
		if err != nil {
			continue
		}
		t.mu.Lock()
		listener := t.udpListener
		stopped := t.stopped
		t.mu.Unlock()
		if stopped || listener == nil {
			return
		}
		if _, err := listener.WriteToUDP(payload, remote); err == nil {
			t.engine.recordBytes(t.id, int64(len(payload)))
		}
	}
}

func (t *tunnel) writeUDPFrame(conn net.Conn, address string, payload []byte) error {
	frame := encodeUDPFrame(address, payload)
	t.udpBridgeWriteMu.Lock()
	defer t.udpBridgeWriteMu.Unlock()
	_, err := conn.Write(frame)
	return err
}

func (t *tunnel) isStopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

func (t *tunnel) closeRuntime() {
	t.mu.Lock()
	minutes := t.flushActiveDurationLocked(time.Now())
	t.activeSince = time.Time{}
	t.stopped = true
	listener := t.tcpListener
	udpListener := t.udpListener
	control := t.control
	udpBridge := t.udpBridge
	sessions := make([]net.Conn, 0, len(t.sessions))
	for session := range t.sessions {
		sessions = append(sessions, session)
	}
	t.tcpListener = nil
	t.udpListener = nil
	t.control = nil
	t.udpBridge = nil
	t.sessions = make(map[net.Conn]struct{})
	t.mu.Unlock()
	t.engine.recordMinutes(t.userID, minutes)
	if listener != nil {
		_ = listener.Close()
	}
	if udpListener != nil {
		_ = udpListener.Close()
	}
	if control != nil {
		_ = control.Close()
	}
	if udpBridge != nil {
		_ = udpBridge.Close()
	}
	for _, session := range sessions {
		_ = session.Close()
	}
}

func (t *tunnel) addSession(connections ...net.Conn) {
	t.mu.Lock()
	for _, connection := range connections {
		t.sessions[connection] = struct{}{}
	}
	t.mu.Unlock()
}

func (t *tunnel) removeSession(connections ...net.Conn) {
	t.mu.Lock()
	for _, connection := range connections {
		delete(t.sessions, connection)
	}
	t.mu.Unlock()
}

func (e *Engine) recordBytes(tunnelID string, bytes int64) {
	if bytes <= 0 {
		return
	}
	e.mu.Lock()
	item := e.tunnels[tunnelID]
	if item == nil {
		e.mu.Unlock()
		return
	}
	usage := e.users[item.userID]
	item.mu.Lock()
	item.unsyncedTransferBytes += bytes
	item.mu.Unlock()
	usage.transferUsed += bytes
	stops := e.enforceUserLimitLocked(usage)
	e.mu.Unlock()
	for _, stopped := range stops {
		stopped.closeRuntime()
	}
}

// CollectUsage prepares an idempotency-free delta batch. Call AcknowledgeUsage
// only after the control plane accepts the complete batch.
func (e *Engine) CollectUsage(now time.Time) []UsageUpdate {
	e.mu.Lock()
	defer e.mu.Unlock()
	var updates []UsageUpdate
	var toStop []*tunnel
	for _, item := range e.tunnels {
		item.mu.Lock()
		minutesAdded := item.flushActiveDurationLocked(now)
		minutes := item.unsyncedMinutes
		bytes := item.unsyncedTransferBytes
		item.mu.Unlock()
		if usage := e.users[item.userID]; usage != nil {
			usage.minutesUsed += minutesAdded
		}
		if minutes != 0 || bytes != 0 {
			updates = append(updates, UsageUpdate{TunnelID: item.id, ActiveMinutes: minutes, TransferBytes: bytes})
		}
	}
	for _, usage := range e.users {
		toStop = append(toStop, e.enforceUserLimitLocked(usage)...)
	}
	go func(items []*tunnel) {
		for _, item := range items {
			item.closeRuntime()
		}
	}(toStop)
	return updates
}

func (e *Engine) AcknowledgeUsage(updates []UsageUpdate) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, update := range updates {
		item := e.tunnels[update.TunnelID]
		if item == nil {
			continue
		}
		item.mu.Lock()
		item.unsyncedMinutes -= min(item.unsyncedMinutes, update.ActiveMinutes)
		item.unsyncedTransferBytes -= min(item.unsyncedTransferBytes, update.TransferBytes)
		item.mu.Unlock()
	}
}

func (e *Engine) StopTunnels(tunnelIDs []string) {
	for _, tunnelID := range tunnelIDs {
		e.mu.RLock()
		item := e.tunnels[tunnelID]
		e.mu.RUnlock()
		if item != nil {
			item.closeRuntime()
		}
	}
}

func (e *Engine) enforceUserLimitLocked(usage *userUsage) []*tunnel {
	minutesExceeded := usage.monthlyMinutesLimit != nil && usage.minutesUsed >= *usage.monthlyMinutesLimit
	transferExceeded := usage.monthlyTransferBytesLimit != nil && usage.transferUsed >= *usage.monthlyTransferBytesLimit
	if !minutesExceeded && !transferExceeded {
		return nil
	}
	items := make([]*tunnel, 0, len(usage.tunnels))
	for _, item := range usage.tunnels {
		item.mu.Lock()
		if !item.stopped {
			item.stopped = true
			items = append(items, item)
		}
		item.mu.Unlock()
	}
	return items
}

func (t *tunnel) flushActiveDurationLocked(now time.Time) int64 {
	if t.activeSince.IsZero() {
		return 0
	}
	seconds := int64(now.Sub(t.activeSince) / time.Second)
	if seconds <= 0 {
		return 0
	}
	t.activeSince = t.activeSince.Add(time.Duration(seconds) * time.Second)
	t.activeRemainderSeconds += seconds
	wholeMinutes := t.activeRemainderSeconds / 60
	if wholeMinutes == 0 {
		return 0
	}
	t.activeRemainderSeconds %= 60
	t.unsyncedMinutes += wholeMinutes
	return wholeMinutes
}

func (e *Engine) recordMinutes(userID string, minutes int64) {
	if minutes == 0 {
		return
	}
	e.mu.Lock()
	usage := e.users[userID]
	if usage != nil {
		usage.minutesUsed += minutes
	}
	var stops []*tunnel
	if usage != nil {
		stops = e.enforceUserLimitLocked(usage)
	}
	e.mu.Unlock()
	for _, stopped := range stops {
		stopped.closeRuntime()
	}
}

func copyLimit(limit *int64) *int64 {
	if limit == nil {
		return nil
	}
	value := *limit
	return &value
}

func relayTCP(left, right net.Conn, record func(int64)) {
	defer left.Close()
	defer right.Close()
	var closeOnce sync.Once
	closeAll := func() {
		closeOnce.Do(func() {
			_ = left.Close()
			_ = right.Close()
		})
	}
	done := make(chan struct{}, 2)
	copyOne := func(destination, source net.Conn) {
		_, err := io.Copy(destination, meteredReader{reader: source, record: record})
		if err != nil {
			closeAll()
		} else if tcp, ok := destination.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}
	go copyOne(right, left)
	go copyOne(left, right)
	<-done
	<-done
}

type meteredReader struct {
	reader io.Reader
	record func(int64)
}

func (r meteredReader) Read(destination []byte) (int, error) {
	n, err := r.reader.Read(destination)
	if n > 0 {
		r.record(int64(n))
	}
	return n, err
}

func encodeUDPFrame(address string, payload []byte) []byte {
	addressBytes := []byte(address)
	frame := make([]byte, 4+len(addressBytes)+4+len(payload))
	putUint32(frame[:4], uint32(len(addressBytes)))
	copy(frame[4:], addressBytes)
	payloadOffset := 4 + len(addressBytes)
	putUint32(frame[payloadOffset:payloadOffset+4], uint32(len(payload)))
	copy(frame[payloadOffset+4:], payload)
	return frame
}

func readUDPFrame(conn net.Conn) (string, []byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", nil, err
	}
	addressLength := int(readUint32(header))
	if addressLength < 1 || addressLength > 256 {
		return "", nil, errors.New("invalid UDP address length")
	}
	address := make([]byte, addressLength)
	if _, err := io.ReadFull(conn, address); err != nil {
		return "", nil, err
	}
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", nil, err
	}
	payloadLength := int(readUint32(header))
	if payloadLength < 0 || payloadLength > 65535 {
		return "", nil, errors.New("invalid UDP payload length")
	}
	payload := make([]byte, payloadLength)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return "", nil, err
	}
	return string(address), payload, nil
}

func putUint32(destination []byte, value uint32) {
	destination[0] = byte(value >> 24)
	destination[1] = byte(value >> 16)
	destination[2] = byte(value >> 8)
	destination[3] = byte(value)
}

func readUint32(source []byte) uint32 {
	return uint32(source[0])<<24 | uint32(source[1])<<16 | uint32(source[2])<<8 | uint32(source[3])
}
