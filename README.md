# rexecd

gRPC remote-exec daemon. Runs as a sidecar inside the fulcrum container and
lets `mattermost-plugin-fulcrum` execute the fulcrum CLI remotely. rexecd has
no fulcrum business knowledge — it is a generic `argv → (exit, stdout, stderr)`
service.

Part of the Mattermost plugin + gRPC remote-exec rewrite. See umbrella
[Mouriya-Emma/fulcrum#221](https://github.com/Mouriya-Emma/fulcrum/issues/221)
for full architecture and threat model.

## Proto v1

`proto/v1/rexec.proto`:

```proto
service RemoteExec {
  rpc Run(RunRequest) returns (RunResponse);
}

message RunRequest {
  repeated string argv         = 1;
  map<string,string> env       = 2;
  string cwd                   = 3;
  uint32 timeout_seconds       = 4;
  // tag 5..19 reserved for streaming / allowlist / client-id / signal_on_cancel
}

message RunResponse {
  int32  exit_code = 1;
  bytes  stdout    = 2;
  bytes  stderr    = 3;
  string error     = 4;
  // tag 5..19 reserved for signal / stream_chunk / rusage / timing
}
```

v1 explicitly does **not** implement mTLS, streaming I/O, or a command
allowlist. Their wire positions are reserved so v2 can add them without
breaking wire compatibility.

### Error model

| Outcome                              | `exit_code` | `error`            |
| ------------------------------------ | ----------- | ------------------ |
| Process exited 0                     | `0`         | empty              |
| Process exited non-zero              | child code  | empty              |
| `timeout_seconds` exceeded           | `-1`        | `timeout after Ns` |
| `argv` empty                         | `-1`        | `argv is empty`    |
| `exec` lookup / fork / setup failure | `-1`        | OS error string    |

Non-zero process exit is **not** a gRPC error — the RPC succeeded; the child
just returned non-zero. Only daemon-level failures populate `error`.

## Build

```bash
make tools          # install protoc-gen-go + protoc-gen-go-grpc
make proto          # regenerate stubs from proto/v1/rexec.proto
make build          # -> bin/rexecd
make test           # go test ./...
make docker-build   # -> rexecd:dev
```

## Run locally

```bash
go run ./cmd/rexecd --listen 127.0.0.1:50051
```

You should see:

```
rexecd listening on 127.0.0.1:50051 pid=<N>
```

## Run via Docker

```bash
docker build -t rexecd:dev .
docker run --rm -p 50051:50051 rexecd:dev
```

The final image is `gcr.io/distroless/static-debian12:nonroot` — no shell, no
libc. The binary is statically linked Go (`CGO_ENABLED=0`).

Note: distroless/static has no `/bin/sh`, so the daemon can only exec
binaries that are themselves present in the image. In the fulcrum sidecar
deployment, rexecd is layered into a fulcrum-CLI image; for standalone
testing, bind-mount the binaries you want to invoke.

## Smoke test

In one terminal:

```bash
go run ./cmd/rexecd --listen 127.0.0.1:50051
```

In another:

```bash
# success path
grpcurl -plaintext -import-path proto/v1 -proto rexec.proto \
  -d '{"argv":["echo","hello"]}' \
  127.0.0.1:50051 rexec.v1.RemoteExec/Run

# non-zero exit
grpcurl -plaintext -import-path proto/v1 -proto rexec.proto \
  -d '{"argv":["sh","-c","exit 7"]}' \
  127.0.0.1:50051 rexec.v1.RemoteExec/Run

# missing binary -> exit_code=-1, error populated
grpcurl -plaintext -import-path proto/v1 -proto rexec.proto \
  -d '{"argv":["definitely-not-a-binary"]}' \
  127.0.0.1:50051 rexec.v1.RemoteExec/Run
```

`stdout` / `stderr` are returned as base64-encoded bytes in the JSON
response; decode with `| jq -r '.stdout' | base64 -d` if you want the raw
bytes.

## Repo layout

```
proto/v1/rexec.proto          # service + reserved tags
proto/v1/rexec.pb.go          # generated (checked in)
proto/v1/rexec_grpc.pb.go     # generated (checked in)
server/server.go              # RemoteExec implementation
server/server_test.go         # bufconn-backed integration tests
cmd/rexecd/main.go            # daemon entrypoint
Dockerfile                    # multi-stage -> distroless/static
Makefile                      # proto / build / docker-build / test
```
