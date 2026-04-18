# waitfor Implementation Spec

## Scope

`waitfor` is a Go CLI that polls one or more semantic conditions until they are
satisfied, a timeout expires, or an unrecoverable condition error occurs.

Runs have one final status:

| Status | Meaning |
| ------ | ------- |
| `satisfied` | Required condition mode completed successfully. |
| `timeout` | The global deadline expired. |
| `cancelled` | The parent context was cancelled, including SIGINT/SIGTERM from the CLI entrypoint. |
| `fatal` | A condition reported an unrecoverable error. |

## Current Backends

| Backend | Syntax | Notes |
| ------- | ------ | ----- |
| `http` | `waitfor http URL --status 2xx --body-contains ok --jsonpath '.ready == true'` | Supports method, exact status, status classes, headers, request bodies, body substring, body regex, minimal JSON expressions, redirect control, and insecure TLS. |
| `tcp` | `waitfor tcp HOST:PORT` | Opens and closes a TCP connection. |
| `exec` | `waitfor exec --output-contains ok -- COMMAND` | Runs a command with context cancellation and checks exit code, output substring, or JSON expression. Supports cwd, env, and output limits. |
| `file` | `waitfor file PATH exists` | Supports `exists`, `deleted`, `nonempty`, and substring checks. |
| `k8s` | `waitfor k8s deployment/myapp --condition Available` | Uses client-go dynamic client and supports common built-in Kubernetes resources. |

## CLI Grammar

Global flags must appear before the first backend. Multiple conditions are
separated with `--` followed by a backend name. `exec` uses `--` to separate
waitfor's exec flags from the command; after that separator, tokens are passed to
the command unchanged.

```text
waitfor [global-flags] condition [-- condition...]
condition := backend backend-args backend-flags
exec-condition := exec [exec-flags] -- command [args...]
```

Because `-- backend` is the condition separator, an exec command that needs the
literal token pair `-- file`, `-- http`, `-- tcp`, `-- exec`, or `-- k8s` cannot
be followed unambiguously by more waitfor conditions.

## Core Contract

Every backend implements:

```go
type Condition interface {
    Descriptor() Descriptor
    Check(ctx context.Context) Result
}

type Result struct {
    Status CheckStatus
    Detail string
    Err    error
}
```

`Check` must be idempotent, safe to call repeatedly, and return promptly when
its context is cancelled. Retryable failures return `CheckUnsatisfied`; fatal
configuration or spawn failures return `CheckFatal`.

## Runner

The runner owns all polling behavior:

```go
type Config struct {
    Conditions        []condition.Condition
    Timeout           time.Duration
    Interval          time.Duration
    PerAttemptTimeout time.Duration
    Mode              Mode
    OnAttempt         func(AttemptEvent)
}
```

The runner executes conditions concurrently. `ModeAll` waits for every condition
to satisfy. `ModeAny` cancels remaining work after the first satisfied condition.
Fatal condition errors take precedence over satisfaction if both are recorded in
the same run. A per-attempt timeout of zero means each check receives the global
run context directly. If a per-attempt timeout is larger than the global
timeout, the effective per-attempt timeout is normalized to the global timeout.

## Output

Text output is optimized for humans. JSON output is stable for scripts:

```json
{
  "status": "satisfied",
  "satisfied": true,
  "mode": "all",
  "elapsed_seconds": 1.2,
  "timeout_seconds": 300.0,
  "interval_seconds": 2.0,
  "conditions": [
    {
      "backend": "tcp",
      "target": "localhost:5432",
      "name": "tcp localhost:5432",
      "satisfied": true,
      "attempts": 2,
      "elapsed_seconds": 1.0,
      "detail": "connection established"
    }
  ]
}
```

Human-readable progress and text summaries are emitted on stderr. JSON output is
emitted on stdout without progress lines.

## Exit Codes

| Code | Meaning |
| ---- | ------- |
| 0 | Conditions satisfied |
| 1 | Timeout expired |
| 2 | Invalid arguments or configuration |
| 3 | Unrecoverable condition failure |
| 130 | Cancelled by parent context or SIGINT/SIGTERM |

## Growth Rules

New backends should add one condition type, parser wiring, unit tests for `Check`,
and at least one CLI-level test. Backend packages must not own polling loops,
sleeping, output formatting, or process exit behavior.
