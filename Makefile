PROTO       := proto/v1/rexec.proto
BIN_DIR     := bin
BINARY      := $(BIN_DIR)/rexecd
IMAGE       := rexecd:dev
GOPATH_BIN  := $(shell go env GOPATH)/bin

.PHONY: proto build docker-build test tools clean

## Install protoc plugins into $(go env GOPATH)/bin.
tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2

## Regenerate Go gRPC stubs from $(PROTO).
proto:
	PATH="$(GOPATH_BIN):$$PATH" protoc \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO)

## Build the rexecd binary into $(BINARY).
build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/rexecd

## Build the rexecd container image tagged $(IMAGE).
docker-build:
	docker build -t $(IMAGE) .

## Run all Go tests.
test:
	go test ./...

clean:
	rm -rf $(BIN_DIR)
