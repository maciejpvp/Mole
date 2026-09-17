package tunnel

import (
	"context"

	"mole-control-plane/internal/relay"
)

// RelayProvisioner adapts the embedded relay to the tunnel service's
// context-aware provisioning boundary.
type RelayProvisioner struct{ engine *relay.Engine }

func NewRelayProvisioner(engine *relay.Engine) *RelayProvisioner {
	return &RelayProvisioner{engine: engine}
}

func (p *RelayProvisioner) Provision(ctx context.Context, request ProvisionRequest) (ProvisionResponse, error) {
	if err := ctx.Err(); err != nil {
		return ProvisionResponse{}, err
	}
	response, err := p.engine.Provision(relay.ProvisionRequest{
		TunnelID: request.TunnelID, UserID: request.UserID, Protocol: request.Protocol, Token: request.Token,
		MonthlyMinutesLimit: request.MonthlyMinutesLimit, MonthlyTransferBytesLimit: request.MonthlyTransferBytesLimit,
		MonthlyMinutesUsed: request.MonthlyMinutesUsed, MonthlyTransferBytesUsed: request.MonthlyTransferBytesUsed,
	})
	if err != nil {
		return ProvisionResponse{}, err
	}
	return ProvisionResponse{OutboundPort: response.OutboundPort, PublicHost: response.PublicHost, ControlPort: response.ControlPort}, nil
}

func (p *RelayProvisioner) Restore(ctx context.Context, request RestoreRequest) (RestoreResponse, error) {
	if err := ctx.Err(); err != nil {
		return RestoreResponse{}, err
	}
	response, err := p.engine.Restore(relay.RestoreRequest{
		TunnelID: request.TunnelID, UserID: request.UserID, Protocol: request.Protocol,
		TokenHash: request.TokenHash, OutboundPort: request.OutboundPort,
		MonthlyMinutesLimit: request.MonthlyMinutesLimit, MonthlyTransferBytesLimit: request.MonthlyTransferBytesLimit,
		MonthlyMinutesUsed: request.MonthlyMinutesUsed, MonthlyTransferBytesUsed: request.MonthlyTransferBytesUsed,
	})
	if err != nil {
		return RestoreResponse{}, err
	}
	return RestoreResponse{OutboundPort: response.OutboundPort, Rebound: response.Rebound}, nil
}

func (p *RelayProvisioner) Deprovision(ctx context.Context, tunnelID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.engine.Deprovision(tunnelID)
}
