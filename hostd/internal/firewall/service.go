package firewall

import (
	"log/slog"
)

// FirewallService provides a high-level interface for firewall operations.
// It abstracts the underlying nftables client and iptables executor.
type FirewallService struct {
	client   NftablesClient
	executor IptablesExecutor
	logger   *slog.Logger
}

// NewFirewallService creates a new FirewallService with the given dependencies.
func NewFirewallService(client NftablesClient, executor IptablesExecutor, logger *slog.Logger) *FirewallService {
	return &FirewallService{
		client:   client,
		executor: executor,
		logger:   logger,
	}
}

// Client returns the underlying nftables client.
func (s *FirewallService) Client() NftablesClient {
	return s.client
}

// Executor returns the underlying iptables executor.
func (s *FirewallService) Executor() IptablesExecutor {
	return s.executor
}

// Logger returns the underlying logger.
func (s *FirewallService) Logger() *slog.Logger {
	return s.logger
}
