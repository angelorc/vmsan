package firewall

import (
	"errors"
	"fmt"
	"strings"
)

// ValidationError indicates a configuration validation failure
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" && e.Value != nil {
		return fmt.Sprintf("%s %s: %v", e.Field, e.Message, e.Value)
	}
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// Unwrap returns nil for sentinel validation errors
func (e *ValidationError) Unwrap() error {
	return nil
}

// FirewallError indicates a firewall operation failure
type FirewallError struct {
	Op   string // "setup", "teardown", "verify", "cleanup"
	VMId string
	Err  error
}

func (e *FirewallError) Error() string {
	if e.VMId != "" {
		return fmt.Sprintf("firewall %s for VM %s: %v", e.Op, e.VMId, e.Err)
	}
	return fmt.Sprintf("firewall %s: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *FirewallError) Unwrap() error {
	return e.Err
}

// NetNSError indicates a network namespace operation failure
type NetNSError struct {
	Op    string // "enter", "create", "delete"
	NetNS string
	Err   error
}

func (e *NetNSError) Error() string {
	if e.NetNS != "" {
		return fmt.Sprintf("netns %s %s: %v", e.NetNS, e.Op, e.Err)
	}
	return fmt.Sprintf("netns %s: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *NetNSError) Unwrap() error {
	return e.Err
}

// RuleError indicates a rule creation/deletion failure
type RuleError struct {
	Op    string // "add", "delete"
	Chain string
	VMId  string
	Err   error
}

func (e *RuleError) Error() string {
	if e.Chain != "" && e.VMId != "" {
		return fmt.Sprintf("rule %s in chain %s for VM %s: %v", e.Op, e.Chain, e.VMId, e.Err)
	}
	if e.Chain != "" {
		return fmt.Sprintf("rule %s in chain %s: %v", e.Op, e.Chain, e.Err)
	}
	return fmt.Sprintf("rule %s: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *RuleError) Unwrap() error {
	return e.Err
}

// IptablesError indicates an iptables command failure
type IptablesError struct {
	Cmd    string
	Args   []string
	Output string
	Err    error
}

func (e *IptablesError) Error() string {
	if e.Output != "" {
		return fmt.Sprintf("iptables %s %s: %v: %s", e.Cmd, strings.Join(e.Args, " "), e.Err, strings.TrimSpace(e.Output))
	}
	return fmt.Sprintf("iptables %s %s: %v", e.Cmd, strings.Join(e.Args, " "), e.Err)
}

// Unwrap returns the underlying error
func (e *IptablesError) Unwrap() error {
	return e.Err
}

// NotFoundError indicates a resource was not found
type NotFoundError struct {
	Resource string // "table", "chain", "rule"
	Name     string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found", e.Resource, e.Name)
}

// Unwrap returns nil for sentinel not-found errors
func (e *NotFoundError) Unwrap() error {
	return nil
}

// Sentinel errors for common validation failures
var (
	ErrMissingVMId      = &ValidationError{Field: "vmId", Message: "vmId is required"}
	ErrMissingPolicy    = &ValidationError{Field: "policy", Message: "policy is required"}
	ErrInvalidPolicy    = &ValidationError{Field: "policy", Message: "policy must be 'allow-all', 'deny-all', or 'custom'"}
	ErrInvalidIP        = &ValidationError{Field: "ip", Message: "invalid IP address format"}
	ErrInvalidSubnet    = &ValidationError{Field: "subnet", Message: "invalid subnet format"}
	ErrMissingTapDevice = &ValidationError{Field: "tapDevice", Message: "tapDevice is required"}
	ErrMissingNetNS     = &ValidationError{Field: "netNSName", Message: "netNSName is required"}
)

// IsValidationError checks if an error is a ValidationError
func IsValidationError(err error) bool {
	var v *ValidationError
	return errors.As(err, &v)
}

// IsFirewallError checks if an error is a FirewallError
func IsFirewallError(err error) bool {
	var f *FirewallError
	return errors.As(err, &f)
}

// IsNotFound checks if an error is a NotFoundError
func IsNotFound(err error) bool {
	var n *NotFoundError
	return errors.As(err, &n)
}

// IsNetNSError checks if an error is a NetNSError
func IsNetNSError(err error) bool {
	var n *NetNSError
	return errors.As(err, &n)
}

// IsRuleError checks if an error is a RuleError
func IsRuleError(err error) bool {
	var r *RuleError
	return errors.As(err, &r)
}

// IsIptablesError checks if an error is an IptablesError
func IsIptablesError(err error) bool {
	var i *IptablesError
	return errors.As(err, &i)
}
