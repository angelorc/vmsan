package firewall

import "github.com/google/nftables"

// tableName returns the per-VM nftables table name.
func tableName(vmId string) string {
	return "vmsan_" + vmId
}

// findTable returns the named IPv4 table if it exists, or nil if not found.
func findTable(c *nftables.Conn, name string) (*nftables.Table, error) {
	tables, err := c.ListTables()
	if err != nil {
		return nil, err
	}
	for _, t := range tables {
		if t.Name == name && t.Family == nftables.TableFamilyIPv4 {
			return t, nil
		}
	}
	return nil, nil
}
