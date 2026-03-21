BINARY := vmsan-nftables
GATEWAY := vmsan-gateway
INSTALL_DIR := $(HOME)/.vmsan/bin

.PHONY: build build-amd64 build-arm64 build-all gateway gateway-amd64 gateway-arm64 gateway-all install clean test

build:
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o $(BINARY) ./cmd/vmsan-nftables

build-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY)-amd64 ./cmd/vmsan-nftables

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(BINARY)-arm64 ./cmd/vmsan-nftables

build-all: build-amd64 build-arm64

gateway:
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o $(GATEWAY) ./cmd/gateway

gateway-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(GATEWAY)-amd64 ./cmd/gateway

gateway-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(GATEWAY)-arm64 ./cmd/gateway

gateway-all: gateway-amd64 gateway-arm64

install: build gateway
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	cp $(GATEWAY) $(INSTALL_DIR)/$(GATEWAY)

test:
	go test ./...

clean:
	rm -f $(BINARY) $(BINARY)-amd64 $(BINARY)-arm64
	rm -f $(GATEWAY) $(GATEWAY)-amd64 $(GATEWAY)-arm64
