GATEWAY := vmsan-gateway
AGENT := vmsan-agent-host
SERVER := vmsan-server
INSTALL_DIR := $(HOME)/.vmsan/bin

.PHONY: build gateway gateway-amd64 gateway-arm64 gateway-all agent agent-amd64 agent-arm64 agent-all server server-amd64 server-arm64 server-all install clean test

gateway:
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o $(GATEWAY) ./cmd/vmsan-gateway

gateway-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(GATEWAY)-amd64 ./cmd/vmsan-gateway

gateway-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(GATEWAY)-arm64 ./cmd/vmsan-gateway

gateway-all: gateway-amd64 gateway-arm64

agent:
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o $(AGENT) ./cmd/vmsan-agent-host

agent-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(AGENT)-amd64 ./cmd/vmsan-agent-host

agent-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(AGENT)-arm64 ./cmd/vmsan-agent-host

agent-all: agent-amd64 agent-arm64

server:
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o $(SERVER) ./cmd/vmsan-server

server-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(SERVER)-amd64 ./cmd/vmsan-server

server-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(SERVER)-arm64 ./cmd/vmsan-server

server-all: server-amd64 server-arm64

build: gateway agent server

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(GATEWAY) $(INSTALL_DIR)/$(GATEWAY)
	cp $(AGENT) $(INSTALL_DIR)/$(AGENT)
	cp $(SERVER) $(INSTALL_DIR)/$(SERVER)

test:
	go test ./...

clean:
	rm -f $(GATEWAY) $(GATEWAY)-amd64 $(GATEWAY)-arm64
	rm -f $(AGENT) $(AGENT)-amd64 $(AGENT)-arm64
	rm -f $(SERVER) $(SERVER)-amd64 $(SERVER)-arm64
