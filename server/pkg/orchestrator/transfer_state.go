package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const transferStateFilename = "global-transfer.json"

// ErrGlobalFuseTripped indicates that a persisted global transfer fuse has
// already stopped this relay. The process must not open any listeners.
var ErrGlobalFuseTripped = errors.New("global transfer fuse is tripped")

type transferState struct {
	TransferBytes int64  `json:"transfer_bytes"`
	Tripped       bool   `json:"tripped"`
	TrippedAt     string `json:"tripped_at,omitempty"`
}

type transferStateStore struct {
	path string
}

func openTransferState(dir string, limit int64) (*transferStateStore, transferState, error) {
	if limit <= 0 {
		return nil, transferState{}, errors.New("global transfer limit must be positive")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, transferState{}, fmt.Errorf("create transfer state directory: %w", err)
	}
	store := &transferStateStore{path: filepath.Join(dir, transferStateFilename)}
	state, err := store.load()
	if err != nil {
		return nil, transferState{}, err
	}
	if state.TransferBytes < 0 {
		return nil, transferState{}, errors.New("global transfer state has a negative counter")
	}
	if state.Tripped {
		return store, state, ErrGlobalFuseTripped
	}
	if state.TransferBytes >= limit {
		state.Tripped = true
		state.TrippedAt = nowUTCString()
		if err := store.save(state); err != nil {
			return nil, transferState{}, err
		}
		return store, state, ErrGlobalFuseTripped
	}
	return store, state, nil
}

func (s *transferStateStore) load() (transferState, error) {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return transferState{}, nil
	}
	if err != nil {
		return transferState{}, fmt.Errorf("open transfer state: %w", err)
	}
	defer file.Close()
	var state transferState
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&state); err != nil {
		return transferState{}, fmt.Errorf("decode transfer state: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return transferState{}, errors.New("decode transfer state: trailing data")
		}
		return transferState{}, fmt.Errorf("decode transfer state: %w", err)
	}
	return state, nil
}

// save replaces the state atomically and fsyncs both the file and directory.
// It is called synchronously for every metered transfer increment.
func (s *transferStateStore) save(state transferState) error {
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".global-transfer-*.tmp")
	if err != nil {
		return fmt.Errorf("create transfer state temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("set transfer state permissions: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		temporary.Close()
		return fmt.Errorf("encode transfer state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync transfer state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close transfer state: %w", err)
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return fmt.Errorf("replace transfer state: %w", err)
	}
	directory, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		return fmt.Errorf("open transfer state directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync transfer state directory: %w", err)
	}
	return nil
}

func nowUTCString() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
}
