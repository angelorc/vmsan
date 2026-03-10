BINARY := vmsan-nftables
INSTALL_DIR := $(HOME)/.vmsan/bin

.PHONY: build build-amd64 build-arm64 build-all install clean test

build:
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o $(BINARY) ./cmd/vmsan-nftables

build-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY)-amd64 ./cmd/vmsan-nftables

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(BINARY)-arm64 ./cmd/vmsan-nftables

build-all: build-amd64 build-arm64

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)

test:
	go test ./...

clean:
	rm -f $(BINARY) $(BINARY)-amd64 $(BINARY)-arm64
