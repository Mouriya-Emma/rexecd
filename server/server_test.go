package server

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	rexecv1 "github.com/Mouriya-Emma/rexecd/proto/v1"
)

func newTestClient(t *testing.T) rexecv1.RemoteExecClient {
	t.Helper()

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	rexecv1.RegisterRemoteExecServer(gs, New())
	go func() {
		if err := gs.Serve(lis); err != nil {
			t.Logf("test server stopped: %v", err)
		}
	}()
	t.Cleanup(func() {
		gs.Stop()
		_ = lis.Close()
	})

	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient(
		"passthrough://bufconn",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return rexecv1.NewRemoteExecClient(conn)
}

func TestRun_SuccessEcho(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Run(ctx, &rexecv1.RunRequest{Argv: []string{"echo", "hello"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0; stderr=%q error=%q", resp.ExitCode, resp.Stderr, resp.Error)
	}
	if strings.TrimSpace(string(resp.Stdout)) != "hello" {
		t.Errorf("stdout = %q, want %q", resp.Stdout, "hello\n")
	}
	if resp.Error != "" {
		t.Errorf("error = %q, want empty", resp.Error)
	}
}

func TestRun_NonZeroExit(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Run(ctx, &rexecv1.RunRequest{Argv: []string{"sh", "-c", "exit 7"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.ExitCode != 7 {
		t.Errorf("exit_code = %d, want 7", resp.ExitCode)
	}
	if resp.Error != "" {
		t.Errorf("error = %q, want empty (non-zero exit is RPC-success)", resp.Error)
	}
}

func TestRun_StderrCaptured(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Run(ctx, &rexecv1.RunRequest{Argv: []string{"sh", "-c", "echo out; echo err 1>&2; exit 0"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", resp.ExitCode)
	}
	if strings.TrimSpace(string(resp.Stdout)) != "out" {
		t.Errorf("stdout = %q", resp.Stdout)
	}
	if strings.TrimSpace(string(resp.Stderr)) != "err" {
		t.Errorf("stderr = %q", resp.Stderr)
	}
}

func TestRun_NotFound(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Run(ctx, &rexecv1.RunRequest{Argv: []string{"this-binary-does-not-exist-rexecd-test"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.ExitCode != -1 {
		t.Errorf("exit_code = %d, want -1", resp.ExitCode)
	}
	if resp.Error == "" {
		t.Error("error = empty, want exec lookup failure description")
	}
}

func TestRun_Timeout(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Run(ctx, &rexecv1.RunRequest{
		Argv:           []string{"sh", "-c", "sleep 3"},
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.ExitCode != -1 {
		t.Errorf("exit_code = %d, want -1", resp.ExitCode)
	}
	if !strings.Contains(resp.Error, "timeout") {
		t.Errorf("error = %q, want contains \"timeout\"", resp.Error)
	}
}

func TestRun_EmptyArgv(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Run(ctx, &rexecv1.RunRequest{Argv: nil})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.ExitCode != -1 || resp.Error == "" {
		t.Errorf("got exit=%d error=%q; want -1 + non-empty error", resp.ExitCode, resp.Error)
	}
}

func TestRun_EnvAndCwd(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Run(ctx, &rexecv1.RunRequest{
		Argv: []string{"sh", "-c", "printf %s \"$REXEC_TEST_VAR\"; printf @; pwd"},
		Env:  map[string]string{"REXEC_TEST_VAR": "tagged", "PATH": "/usr/bin:/bin"},
		Cwd:  "/tmp",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.ExitCode != 0 {
		t.Fatalf("exit_code = %d stderr=%q error=%q", resp.ExitCode, resp.Stderr, resp.Error)
	}
	got := strings.TrimSpace(string(resp.Stdout))
	// `pwd` on macOS may report /private/tmp; accept either form.
	if !strings.HasPrefix(got, "tagged@") || !(strings.HasSuffix(got, "/tmp") || strings.HasSuffix(got, "/private/tmp")) {
		t.Errorf("stdout = %q, want tagged@<resolved /tmp>", got)
	}
}
