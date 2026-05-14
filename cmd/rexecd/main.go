// Command rexecd starts the rexec.v1.RemoteExec gRPC daemon.
package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	rexecv1 "github.com/Mouriya-Emma/rexecd/proto/v1"
	"github.com/Mouriya-Emma/rexecd/server"
)

func main() {
	listen := flag.String("listen", ":50051", "TCP address to listen on")
	flag.Parse()

	l, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("listen %s: %v", *listen, err)
	}

	gs := grpc.NewServer()
	rexecv1.RegisterRemoteExecServer(gs, server.New())

	log.Printf("rexecd listening on %s pid=%d", l.Addr(), os.Getpid())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received %s, gracefully stopping", sig)
		gs.GracefulStop()
	}()

	if err := gs.Serve(l); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
