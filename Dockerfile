# syntax=docker/dockerfile:1.7

# ---- builder ----
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache modules independently of source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static binary so distroless/static can host it without libc.
ARG TARGETOS=linux
ARG TARGETARCH=arm64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/rexecd ./cmd/rexecd

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/rexecd /rexecd

EXPOSE 50051

USER nonroot:nonroot
ENTRYPOINT ["/rexecd"]
CMD ["--listen", ":50051"]
