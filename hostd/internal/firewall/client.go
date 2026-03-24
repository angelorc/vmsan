package firewall

import "github.com/google/nftables"

// NftablesClient defines the interface for nftables operations.
// This abstraction allows for dependency injection and easier testing.
type NftablesClient interface {
	AddTable(*nftables.Table) *nftables.Table
	DelTable(*nftables.Table)
	AddChain(*nftables.Chain) *nftables.Chain
	DelChain(*nftables.Chain)
	AddRule(*nftables.Rule) *nftables.Rule
	DelRule(*nftables.Rule) error
	ListTables() ([]*nftables.Table, error)
	ListChains() ([]*nftables.Chain, error)
	GetRules(*nftables.Table, *nftables.Chain) ([]*nftables.Rule, error)
	Flush() error
	FlushRuleset()
}

// Compile-time check: ensure *nftables.Conn implements NftablesClient.
var _ NftablesClient = (*nftables.Conn)(nil)
