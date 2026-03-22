module github.com/angelorc/vmsan/hostd

go 1.25.0

require (
	github.com/angelorc/vmsan/nftables v0.0.0-00010101000000-000000000000
	github.com/miekg/dns v1.1.72
	inet.af/tcpproxy v0.0.0-20231102063150-2862066fc2a9
	modernc.org/sqlite v1.47.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	modernc.org/libc v1.70.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/angelorc/vmsan/nftables => ../nftables
