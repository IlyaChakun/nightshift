package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "no args", args: nil, wantCode: exitUsage, wantStderr: "Usage:"},
		{name: "help", args: []string{"--help"}, wantCode: exitOK, wantStdout: "Commands:"},
		{name: "version", args: []string{"version"}, wantCode: exitOK, wantStdout: "nightshift dev"},
		{name: "stub", args: []string{"standards"}, wantCode: exitNotImplemented, wantStderr: "not implemented yet (design phase 4)"},
		{name: "work without secrets", args: []string{"work", "--secrets", "/nonexistent"}, wantCode: exitFailure, wantStderr: "worker_token"},
		{name: "status without flags", args: []string{"status"}, wantCode: exitNotImplemented, wantStderr: "only --health"},
		{name: "unhealthy", args: []string{"status", "--health", "--addr", "127.0.0.1:1"}, wantCode: exitUnhealthy, wantStderr: "unhealthy"},
		{name: "serve without config", args: []string{"serve", "--config", "/nonexistent"}, wantCode: exitFailure, wantStderr: "serve failed"},
		{name: "migrate without secrets", args: []string{"serve", "--migrate-only", "--secrets", "/nonexistent"}, wantCode: exitFailure, wantStderr: "pg_owner"},
		{name: "unknown", args: []string{"deploy"}, wantCode: exitUsage, wantStderr: `unknown command "deploy"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestEveryCommandIsListedInUsage(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	usage(&out)
	for _, c := range commands {
		if !strings.Contains(out.String(), "  "+c.name+" ") {
			t.Errorf("usage does not list %q", c.name)
		}
	}
}
