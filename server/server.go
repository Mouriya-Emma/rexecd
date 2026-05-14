// Package server implements the rexec.v1.RemoteExec gRPC service.
//
// v1 scope: synchronous unary Run that shells out to argv[0] with argv[1:]
// under a context-bound timeout, buffers stdout/stderr, and reports
// exit_code + error string. No mTLS, no streaming, no command allowlist.
package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	rexecv1 "github.com/Mouriya-Emma/rexecd/proto/v1"
)

type Server struct {
	rexecv1.UnimplementedRemoteExecServer
}

func New() *Server { return &Server{} }

// Run executes req.Argv under an optional timeout. A non-zero exit from the
// child process is reported via RunResponse.exit_code, not as a gRPC error —
// the RPC itself is considered successful as long as the daemon was able to
// attempt the exec. Daemon-level failures (argv empty, exec lookup, timeout)
// are reported via RunResponse.error with exit_code=-1.
func (s *Server) Run(ctx context.Context, req *rexecv1.RunRequest) (*rexecv1.RunResponse, error) {
	if req == nil || len(req.Argv) == 0 {
		return &rexecv1.RunResponse{ExitCode: -1, Error: "argv is empty"}, nil
	}

	runCtx := ctx
	if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, req.Argv[0], req.Argv[1:]...)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	if len(req.Env) > 0 {
		envSlice := make([]string, 0, len(req.Env))
		for k, v := range req.Env {
			envSlice = append(envSlice, k+"="+v)
		}
		cmd.Env = envSlice
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()

	resp := &rexecv1.RunResponse{
		Stdout: stdoutBuf.Bytes(),
		Stderr: stderrBuf.Bytes(),
	}

	switch {
	case runErr == nil:
		resp.ExitCode = 0
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		resp.ExitCode = -1
		resp.Error = fmt.Sprintf("timeout after %ds", req.TimeoutSeconds)
	default:
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			resp.ExitCode = int32(exitErr.ExitCode())
		} else {
			resp.ExitCode = -1
			resp.Error = runErr.Error()
		}
	}

	return resp, nil
}
