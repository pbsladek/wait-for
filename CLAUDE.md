# wait-for

`waitfor` is a semantic condition poller — it blocks until one or more conditions are satisfied, then exits 0. It is used in shell scripts, CI pipelines, Kubernetes init containers, and agent workflows.

## Build & Test

```bash
make build       # go build -o bin/waitfor ./cmd/waitfor
make test        # go test ./...
make lint        # golangci-lint run
make coverage    # go test -coverpkg=./... then open coverage.html
```

Verification before finishing any change:

```bash
go build ./... && go test ./... && golangci-lint run
gocyclo -over 9 $(find . -name '*.go' -not -name '*_test.go')
```

## Architecture

```
cmd/waitfor/        — Cobra entrypoint; delegates to internal/cli
internal/cli/       — Multi-condition argument parsing; backend wiring
internal/condition/ — One file per backend: http, tcp, dns, docker, file, exec, k8s
internal/runner/    — Timeout, interval, all/any mode, structured output
internal/output/    — text/json formatters (human progress → stderr, JSON → stdout)
internal/expr/      — Minimal JSONPath expression evaluator used by http, exec, and k8s
```

Key interface — every backend implements this in `internal/condition/condition.go`:

```go
type Condition interface {
    Descriptor() Descriptor
    Check(ctx context.Context) Result
}
```

`Check` returns one of three statuses: `satisfied`, `unsatisfied`, or `fatal`. The runner maps these to exit codes 0, 1, and 3 respectively.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0    | All (or any) conditions satisfied |
| 1    | Timeout expired |
| 2    | Invalid arguments |
| 3    | Fatal condition failure |
| 130  | Cancelled (SIGINT/SIGTERM) |

## Adding a Backend

1. Create `internal/condition/<name>.go` implementing `condition.Condition`.
2. Add parser wiring in `internal/cli/cli.go`.
3. Add table-driven tests in `internal/condition/<name>_test.go`.
4. Add e2e coverage in `e2e/e2e_test.go` for satisfied, timeout, invalid args, and fatal paths where applicable.
5. Backends must return promptly when `ctx` is cancelled.
6. Use `condition.Satisfied`, `condition.Unsatisfied`, or `condition.Fatal` helpers rather than constructing `Result` directly.
7. Keep every production function at `gocyclo` score 9 or below by extracting package-level helpers.

## Design Constraints

- CLI parsing is separate from backend implementations — backends must be testable without Cobra.
- The runner is backend-agnostic; all/any mode, timeout, and parallelism live there.
- JSON output goes to stdout only; human progress goes to stderr.
- Kubernetes uses a getter abstraction so tests use client-go fakes; production uses the dynamic client.
- DNS defaults to the stdlib resolver. Use the `codeberg.org/miekg/dns` v2 wire resolver only when precise DNS message behavior is required, such as `NXDOMAIN` vs `NODATA`, response codes, transport selection, EDNS0, or record types outside the stdlib path.
- Docker polling uses the Docker CLI. A missing Docker binary is fatal; missing containers and non-matching states are retryable.
- JSON expression support stays minimal unless a concrete use case requires expansion.
- Do not add dependencies that stdlib or existing deps already cover. New dependencies need a concrete capability gap, as with DNS wire-level checks.
