package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteFileJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready")
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{"--output", "json", "file", path, "exists"}, nil, &stdout, &stderr)
	if code != ExitSatisfied {
		t.Fatalf("exit code = %d, want %d, stderr = %q", code, ExitSatisfied, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty for JSON output", stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v: %s", err, stdout.String())
	}
	if payload["satisfied"] != true {
		t.Fatalf("satisfied = %v, want true", payload["satisfied"])
	}
}

func TestExecuteTextWritesProgressToStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready")
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{"file", path, "exists"}, nil, &stdout, &stderr)
	if code != ExitSatisfied {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitSatisfied, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty for text output", stdout.String())
	}
	if stderr.Len() == 0 {
		t.Fatal("stderr is empty, want text progress")
	}
}

func TestExecuteTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{"--timeout", "20ms", "--interval", "5ms", "file", path, "exists"}, nil, &stdout, &stderr)
	if code != ExitTimeout {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitTimeout, stdout.String(), stderr.String())
	}
}

func TestExecuteCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	path := filepath.Join(t.TempDir(), "missing")
	var stdout, stderr bytes.Buffer
	code := Execute(ctx, []string{"--timeout", "1s", "--interval", "5ms", "file", path, "exists"}, nil, &stdout, &stderr)
	if code != ExitCancelled {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitCancelled, stdout.String(), stderr.String())
	}
}

func TestExecuteHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Smoke"); got != "yes" {
			t.Fatalf("X-Smoke = %q, want yes", got)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "ping" {
			t.Fatalf("body = %q, want ping", string(body))
		}
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"ready":true,"message":"ok"}`)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{
		"--output", "json",
		"http", server.URL,
		"--method", "POST",
		"--status", "2xx",
		"--body", "ping",
		"--body-contains", "ok",
		"--body-matches", `"message":"ok"`,
		"--jsonpath", ".ready == true",
		"--header", "X-Smoke=yes",
	}, nil, &stdout, &stderr)
	if code != ExitSatisfied {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitSatisfied, stdout.String(), stderr.String())
	}
}

func TestExecuteHTTPBodyFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "from-file" {
			t.Fatalf("body = %q, want from-file", string(body))
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	bodyPath := filepath.Join(t.TempDir(), "body.txt")
	if err := os.WriteFile(bodyPath, []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{
		"http", server.URL,
		"--method", "POST",
		"--body-file", bodyPath,
		"--body-contains", "ok",
	}, nil, &stdout, &stderr)
	if code != ExitSatisfied {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitSatisfied, stdout.String(), stderr.String())
	}
}

func TestExecuteTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan struct{})
	go func() {
		defer close(accepted)
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{"tcp", listener.Addr().String()}, nil, &stdout, &stderr)
	if code != ExitSatisfied {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitSatisfied, stdout.String(), stderr.String())
	}
	<-accepted
}

func TestExecuteModeAnyWithMultipleConditions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready")
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing")

	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{
		"--timeout", "100ms",
		"--interval", "5ms",
		"--mode", "any",
		"file", path, "exists",
		"--", "file", missing, "exists",
	}, nil, &stdout, &stderr)
	if code != ExitSatisfied {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitSatisfied, stdout.String(), stderr.String())
	}
}

func TestExecuteExecRequiresFlagsBeforeSeparator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{
		"exec", "--output-contains", "ready", "--", "/bin/sh", "-c", "printf ready",
	}, nil, &stdout, &stderr)
	if code != ExitSatisfied {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitSatisfied, stdout.String(), stderr.String())
	}
}

func TestExecuteExecCwdEnvAndOutputLimit(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{
		"exec",
		"--cwd", dir,
		"--env", "WAITFOR_TEST=yes",
		"--max-output-bytes", fmt.Sprint(len(dir) + len(":yes")),
		"--output-contains", ":yes",
		"--", "/bin/sh", "-c", "printf '%s:%s:long-output' \"$PWD\" \"$WAITFOR_TEST\"",
	}, nil, &stdout, &stderr)
	if code != ExitSatisfied {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitSatisfied, stdout.String(), stderr.String())
	}
}

func TestExecuteExecCommandHelpDoesNotTriggerWaitforHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{
		"exec", "--output-contains", "usage", "--", "/bin/sh", "-c", "printf usage --help",
	}, nil, &stdout, &stderr)
	if code != ExitSatisfied {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitSatisfied, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "semantic condition poller") || strings.Contains(stderr.String(), "semantic condition poller") {
		t.Fatalf("waitfor help was printed unexpectedly, stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestExecuteExecDoesNotParseFlagsAfterSeparator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{
		"--timeout", "20ms",
		"--interval", "5ms",
		"exec", "--", "/bin/sh", "-c", "exit 1", "--exit-code", "1",
	}, nil, &stdout, &stderr)
	if code != ExitTimeout {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitTimeout, stdout.String(), stderr.String())
	}
}

func TestExecuteInvalidArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{"tcp", "not-a-port"}, nil, &stdout, &stderr)
	if code != ExitInvalid {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitInvalid, stdout.String(), stderr.String())
	}
}

func TestSplitConditionSegments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "single", args: []string{"file", "README.md", "exists"}, want: 1},
		{name: "multiple", args: []string{"file", "README.md", "exists", "--", "tcp", "127.0.0.1:1"}, want: 2},
		{name: "bare separator inside exec command", args: []string{"exec", "--", "/bin/echo", "--", "not-a-backend"}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitConditionSegments(tt.args)
			if err != nil {
				t.Fatalf("splitConditionSegments() error = %v", err)
			}
			if len(got) != tt.want {
				t.Fatalf("len(splitConditionSegments()) = %d, want %d: %#v", len(got), tt.want, got)
			}
		})
	}
}

func TestExecuteParserEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "trailing separator", args: []string{"file", "README.md", "exists", "--"}},
		{name: "empty segment", args: []string{"--", "file", "README.md", "exists"}},
		{name: "unknown backend", args: []string{"nope", "target"}},
		{name: "global flag after backend", args: []string{"file", "README.md", "exists", "--timeout", "1s"}},
		{name: "exec missing separator", args: []string{"exec", "/bin/echo", "ready"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Execute(t.Context(), tt.args, nil, &stdout, &stderr)
			if code != ExitInvalid {
				t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitInvalid, stdout.String(), stderr.String())
			}
		})
	}
}

func TestExecuteMalformedGlobalFlagReportsFlagError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(t.Context(), []string{"--timeout", "file", "README.md", "exists"}, nil, &stdout, &stderr)
	if code != ExitInvalid {
		t.Fatalf("exit code = %d, want %d, stdout = %q, stderr = %q", code, ExitInvalid, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--timeout") {
		t.Fatalf("stderr = %q, want timeout flag error", stderr.String())
	}
}
