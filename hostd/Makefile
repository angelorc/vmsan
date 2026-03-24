BINARY := vmsan
INSTALL_DIR := $(HOME)/.vmsan/bin
VERSION ?= dev

.PHONY: build build-amd64 build-arm64 build-all install clean test proto

build:
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/vmsan

build-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY)-amd64 ./cmd/vmsan

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY)-arm64 ./cmd/vmsan

build-all: build-amd64 build-arm64

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)

proto:
	protoc --proto_path=proto \
		--go_out=gen --go_opt=paths=source_relative \
		--go-grpc_out=gen --go-grpc_opt=paths=source_relative \
		proto/vmsan/v1/gateway.proto

test:
	go test ./...

clean:
	rm -f $(BINARY) $(BINARY)-amd64 $(BINARY)-arm64
