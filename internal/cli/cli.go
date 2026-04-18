package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pbsladek/wait-for/internal/condition"
	"github.com/pbsladek/wait-for/internal/expr"
	"github.com/pbsladek/wait-for/internal/output"
	"github.com/pbsladek/wait-for/internal/runner"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	ExitSatisfied = 0
	ExitTimeout   = 1
	ExitInvalid   = 2
	ExitFatal     = 3
	ExitCancelled = 130
)

type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e exitError) Unwrap() error {
	return e.err
}

func Execute(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	cmd := newCommand(stdin, stdout, stderr)
	cmd.SetArgs(args)
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		var ee exitError
		if errors.As(err, &ee) {
			if ee.err != nil {
				fmt.Fprintf(stderr, "waitfor: %v\n", ee.err)
			}
			return ee.code
		}
		fmt.Fprintf(stderr, "waitfor: %v\n", err)
		return ExitFatal
	}
	return ExitSatisfied
}

func newCommand(stdin io.Reader, stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "waitfor [flags] <backend> <target> [backend-flags] [-- <backend> ...]",
		Short:              "Wait until semantic conditions are satisfied",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if wantsHelp(args) {
				_, _ = io.WriteString(stdout, helpText())
				return nil
			}
			code, err := run(cmd.Context(), args, stdout, stderr)
			if err != nil {
				return exitError{code: code, err: err}
			}
			if code != ExitSatisfied {
				return exitError{code: code}
			}
			return nil
		},
	}
	cmd.SetIn(stdin)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd
}

type globalOptions struct {
	timeout           time.Duration
	interval          time.Duration
	perAttemptTimeout time.Duration
	format            output.Format
	mode              runner.Mode
	verbose           bool
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) (int, error) {
	opts, rest, err := parseGlobal(args)
	if err != nil {
		return ExitInvalid, err
	}
	conditions, err := parseConditions(rest)
	if err != nil {
		return ExitInvalid, err
	}

	outputWriter := stderr
	if opts.format == output.FormatJSON {
		outputWriter = stdout
	}
	printer := output.NewPrinter(outputWriter, opts.format, opts.verbose)
	printer.Start(len(conditions), opts.timeout, opts.interval)
	out, err := runner.Run(ctx, runner.Config{
		Conditions:        conditions,
		Timeout:           opts.timeout,
		Interval:          opts.interval,
		PerAttemptTimeout: opts.perAttemptTimeout,
		Mode:              opts.mode,
		OnAttempt: func(event runner.AttemptEvent) {
			printer.Attempt(output.Attempt{
				Name:      event.Name,
				Attempt:   event.Attempt,
				Satisfied: event.Satisfied,
				Detail:    event.Detail,
				Error:     event.Error,
				Elapsed:   event.Elapsed,
			})
		},
	})
	if err != nil {
		return ExitInvalid, err
	}
	if err := printer.Outcome(reportFromOutcome(out)); err != nil {
		return ExitFatal, err
	}
	switch out.Status {
	case runner.StatusSatisfied:
		return ExitSatisfied, nil
	case runner.StatusFatal:
		return ExitFatal, nil
	case runner.StatusCancelled:
		return ExitCancelled, nil
	default:
		return ExitTimeout, nil
	}
}

func parseGlobal(args []string) (globalOptions, []string, error) {
	opts := globalOptions{
		timeout:  5 * time.Minute,
		interval: 2 * time.Second,
		format:   output.FormatText,
		mode:     runner.ModeAll,
	}

	idx := firstBackendIndex(args)
	if idx < 0 {
		return opts, nil, fmt.Errorf("missing condition backend")
	}

	fs := pflag.NewFlagSet("waitfor", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var format string
	var mode string
	fs.DurationVar(&opts.timeout, "timeout", opts.timeout, "global deadline")
	fs.DurationVar(&opts.interval, "interval", opts.interval, "poll interval")
	fs.DurationVar(&opts.perAttemptTimeout, "attempt-timeout", 0, "per-attempt deadline; 0 uses the global remaining time")
	fs.StringVar(&format, "output", string(opts.format), "output format: text|json")
	fs.StringVar(&mode, "mode", "all", "condition mode: all|any")
	fs.BoolVar(&opts.verbose, "verbose", false, "show every attempt")
	if err := fs.Parse(args[:idx]); err != nil {
		return opts, nil, err
	}
	if len(fs.Args()) != 0 {
		return opts, nil, fmt.Errorf("unexpected global arguments: %s", strings.Join(fs.Args(), " "))
	}
	switch output.Format(format) {
	case output.FormatText, output.FormatJSON:
		opts.format = output.Format(format)
	default:
		return opts, nil, fmt.Errorf("invalid output format %q", format)
	}
	switch mode {
	case "all":
		opts.mode = runner.ModeAll
	case "any":
		opts.mode = runner.ModeAny
	default:
		return opts, nil, fmt.Errorf("invalid mode %q", mode)
	}
	if opts.timeout <= 0 {
		return opts, nil, fmt.Errorf("timeout must be positive")
	}
	if opts.interval <= 0 {
		return opts, nil, fmt.Errorf("interval must be positive")
	}
	if opts.perAttemptTimeout < 0 {
		return opts, nil, fmt.Errorf("attempt-timeout cannot be negative")
	}
	return opts, args[idx:], nil
}

func parseConditions(args []string) ([]condition.Condition, error) {
	segments, err := splitConditionSegments(args)
	if err != nil {
		return nil, err
	}
	conditions := make([]condition.Condition, 0, len(segments))
	for _, segment := range segments {
		cond, err := parseCondition(segment)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, cond)
	}
	return conditions, nil
}

func parseCondition(segment []string) (condition.Condition, error) {
	if len(segment) == 0 {
		return nil, fmt.Errorf("empty condition")
	}
	switch segment[0] {
	case "http":
		return parseHTTPCondition(segment)
	case "tcp":
		return parseTCPCondition(segment)
	case "exec":
		return parseExecCondition(segment)
	case "file":
		return parseFileCondition(segment)
	case "k8s":
		return parseKubernetesCondition(segment)
	default:
		return nil, fmt.Errorf("unknown backend %q", segment[0])
	}
}

func parseHTTPCondition(segment []string) (condition.Condition, error) {
	fs := pflag.NewFlagSet("http", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	method := "GET"
	status := "200"
	body := ""
	bodyFile := ""
	bodyContains := ""
	bodyMatches := ""
	jsonpath := ""
	insecure := false
	noRedirects := false
	headers := []string{}
	fs.StringVar(&method, "method", method, "HTTP method")
	fs.StringVar(&status, "status", status, "expected HTTP status or class, such as 200 or 2xx")
	fs.StringVar(&body, "body", "", "request body")
	fs.StringVar(&bodyFile, "body-file", "", "request body file")
	fs.StringVar(&bodyContains, "body-contains", bodyContains, "required body substring")
	fs.StringVar(&bodyMatches, "body-matches", bodyMatches, "required body regex")
	fs.StringVar(&jsonpath, "jsonpath", jsonpath, "JSON expression")
	fs.BoolVar(&insecure, "insecure", insecure, "skip TLS verification")
	fs.BoolVar(&noRedirects, "no-follow-redirects", noRedirects, "do not follow HTTP redirects")
	fs.StringArrayVar(&headers, "header", nil, "request header, as Key: Value or Key=Value")
	if err := fs.Parse(segment[1:]); err != nil {
		return nil, err
	}
	args := fs.Args()
	if len(args) != 1 {
		return nil, fmt.Errorf("http requires exactly one URL")
	}
	parsed, err := url.Parse(args[0])
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid http URL %q", args[0])
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("http URL must use http or https")
	}
	statusMatcher, err := condition.ParseHTTPStatusMatcher(status)
	if err != nil {
		return nil, err
	}
	if body != "" && bodyFile != "" {
		return nil, fmt.Errorf("--body and --body-file are mutually exclusive")
	}
	var requestBody []byte
	if body != "" {
		requestBody = []byte(body)
	}
	if bodyFile != "" {
		requestBody, err = os.ReadFile(bodyFile)
		if err != nil {
			return nil, fmt.Errorf("read body file: %w", err)
		}
	}
	var bodyRegex *regexp.Regexp
	if bodyMatches != "" {
		bodyRegex, err = regexp.Compile(bodyMatches)
		if err != nil {
			return nil, fmt.Errorf("invalid body regex: %w", err)
		}
	}
	var bodyExpr *expr.Expression
	if jsonpath != "" {
		bodyExpr, err = expr.Compile(jsonpath)
		if err != nil {
			return nil, err
		}
	}
	cond := condition.NewHTTP(args[0])
	cond.Method = method
	cond.StatusMatcher = statusMatcher
	cond.RequestBody = requestBody
	cond.BodyContains = bodyContains
	cond.BodyMatches = bodyMatches
	cond.BodyRegex = bodyRegex
	cond.BodyJSONPath = jsonpath
	cond.BodyJSONExpr = bodyExpr
	cond.Insecure = insecure
	cond.NoRedirects = noRedirects
	for _, header := range headers {
		key, value, ok := splitHeader(header)
		if !ok {
			return nil, fmt.Errorf("invalid header %q", header)
		}
		cond.Headers[key] = value
	}
	return cond, nil
}

func parseTCPCondition(segment []string) (condition.Condition, error) {
	fs := pflag.NewFlagSet("tcp", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(segment[1:]); err != nil {
		return nil, err
	}
	args := fs.Args()
	if len(args) != 1 {
		return nil, fmt.Errorf("tcp requires exactly one host:port address")
	}
	if _, _, err := net.SplitHostPort(args[0]); err != nil {
		return nil, fmt.Errorf("invalid tcp address %q: %w", args[0], err)
	}
	return condition.NewTCP(args[0]), nil
}

func parseFileCondition(segment []string) (condition.Condition, error) {
	fs := pflag.NewFlagSet("file", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	contains := ""
	fs.StringVar(&contains, "contains", "", "required file substring")
	if err := fs.Parse(segment[1:]); err != nil {
		return nil, err
	}
	args := fs.Args()
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("file requires path and optional state")
	}
	state := condition.FileExists
	if len(args) == 2 {
		state = condition.FileState(args[1])
	}
	switch state {
	case condition.FileExists, condition.FileDeleted, condition.FileNonEmpty:
	default:
		return nil, fmt.Errorf("invalid file state %q", state)
	}
	cond := condition.NewFile(args[0], state)
	cond.Contains = contains
	return cond, nil
}

func parseKubernetesCondition(segment []string) (condition.Condition, error) {
	fs := pflag.NewFlagSet("k8s", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	namespace := "default"
	conditionName := ""
	jsonpath := ""
	kubeconfig := ""
	fs.StringVar(&namespace, "namespace", namespace, "namespace")
	fs.StringVar(&conditionName, "condition", conditionName, "condition type")
	fs.StringVar(&jsonpath, "jsonpath", jsonpath, "JSON expression")
	fs.StringVar(&kubeconfig, "kubeconfig", kubeconfig, "kubeconfig path")
	if err := fs.Parse(segment[1:]); err != nil {
		return nil, err
	}
	args := fs.Args()
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("k8s requires resource and optional condition")
	}
	if len(args) == 2 {
		conditionName = args[1]
	}
	cond := condition.NewKubernetes(args[0])
	cond.Namespace = namespace
	cond.Condition = conditionName
	cond.JSONPath = jsonpath
	cond.Kubeconfig = kubeconfig
	return cond, nil
}

type execOptions struct {
	expectedExitCode int
	outputContains   string
	jsonpath         string
	jsonExpr         *expr.Expression
	cwd              string
	env              []string
	maxOutputBytes   int64
}

func parseExecCondition(segment []string) (condition.Condition, error) {
	tokens := append([]string(nil), segment[1:]...)
	opts := execOptions{}

	separator := indexOf(tokens, "--")
	var command []string
	if separator < 0 {
		return nil, fmt.Errorf("exec requires -- before command")
	}

	before := tokens[:separator]
	after := tokens[separator+1:]
	var err error
	opts, before, err = extractExecFlags(before, opts)
	if err != nil {
		return nil, err
	}
	if len(before) != 0 {
		return nil, fmt.Errorf("exec flags must precede --")
	}
	command = after

	if len(command) == 0 {
		return nil, fmt.Errorf("exec requires a command; use: waitfor exec [flags] -- command [args...]")
	}
	cond := condition.NewExec(command)
	cond.ExpectedExitCode = opts.expectedExitCode
	cond.OutputContains = opts.outputContains
	cond.OutputJSONPath = opts.jsonpath
	cond.OutputJSONExpr = opts.jsonExpr
	cond.Cwd = opts.cwd
	cond.Env = opts.env
	cond.MaxOutputBytes = opts.maxOutputBytes
	return cond, nil
}

func extractExecFlags(tokens []string, opts execOptions) (execOptions, []string, error) {
	remaining := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		name, value, hasValue := strings.Cut(token, "=")
		switch name {
		case "--exit-code":
			if !hasValue {
				i++
				if i >= len(tokens) {
					return opts, nil, fmt.Errorf("--exit-code requires a value")
				}
				value = tokens[i]
			}
			code, err := strconv.Atoi(value)
			if err != nil {
				return opts, nil, fmt.Errorf("invalid --exit-code %q", value)
			}
			opts.expectedExitCode = code
		case "--output-contains":
			if !hasValue {
				i++
				if i >= len(tokens) {
					return opts, nil, fmt.Errorf("--output-contains requires a value")
				}
				value = tokens[i]
			}
			opts.outputContains = value
		case "--jsonpath":
			if !hasValue {
				i++
				if i >= len(tokens) {
					return opts, nil, fmt.Errorf("--jsonpath requires a value")
				}
				value = tokens[i]
			}
			opts.jsonpath = value
			expression, err := expr.Compile(value)
			if err != nil {
				return opts, nil, err
			}
			opts.jsonExpr = expression
		case "--cwd":
			if !hasValue {
				i++
				if i >= len(tokens) {
					return opts, nil, fmt.Errorf("--cwd requires a value")
				}
				value = tokens[i]
			}
			opts.cwd = value
		case "--env":
			if !hasValue {
				i++
				if i >= len(tokens) {
					return opts, nil, fmt.Errorf("--env requires a value")
				}
				value = tokens[i]
			}
			if !strings.Contains(value, "=") {
				return opts, nil, fmt.Errorf("--env must use KEY=VALUE")
			}
			opts.env = append(opts.env, value)
		case "--max-output-bytes":
			if !hasValue {
				i++
				if i >= len(tokens) {
					return opts, nil, fmt.Errorf("--max-output-bytes requires a value")
				}
				value = tokens[i]
			}
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 {
				return opts, nil, fmt.Errorf("invalid --max-output-bytes %q", value)
			}
			opts.maxOutputBytes = n
		default:
			remaining = append(remaining, token)
		}
	}
	return opts, remaining, nil
}

func splitConditionSegments(args []string) ([][]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("missing condition")
	}
	if args[0] == "--" {
		return nil, fmt.Errorf("empty condition before --")
	}
	if args[len(args)-1] == "--" {
		return nil, fmt.Errorf("empty trailing condition")
	}
	var segments [][]string
	var current []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--" && i+1 < len(args) && isBackend(args[i+1]) {
			if len(current) == 0 {
				return nil, fmt.Errorf("empty condition before --")
			}
			segments = append(segments, current)
			current = nil
			continue
		}
		current = append(current, args[i])
	}
	if len(current) == 0 {
		return nil, fmt.Errorf("empty trailing condition")
	}
	segments = append(segments, current)
	return segments, nil
}

func firstBackendIndex(args []string) int {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return -1
		}
		if isBackend(arg) {
			return i
		}
	}
	return -1
}

func isBackend(arg string) bool {
	switch arg {
	case "http", "tcp", "exec", "file", "k8s":
		return true
	default:
		return false
	}
}

func wantsHelp(args []string) bool {
	if len(args) == 0 {
		return true
	}
	return args[0] == "-h" || args[0] == "--help" || args[0] == "help"
}

func reportFromOutcome(out runner.Outcome) output.Report {
	report := output.Report{
		Status:          string(out.Status),
		Satisfied:       out.Satisfied(),
		Mode:            out.Mode.String(),
		ElapsedSeconds:  output.Seconds(out.Elapsed),
		TimeoutSeconds:  output.Seconds(out.Timeout),
		IntervalSeconds: output.Seconds(out.Interval),
		Conditions:      make([]output.ConditionReport, 0, len(out.Conditions)),
	}
	if out.PerAttemptTimeout > 0 {
		report.PerAttemptTimeoutSeconds = output.Seconds(out.PerAttemptTimeout)
	}
	for _, rec := range out.Conditions {
		report.Conditions = append(report.Conditions, output.ConditionReport{
			Backend:        rec.Backend,
			Target:         rec.Target,
			Name:           rec.Name,
			Satisfied:      rec.Satisfied,
			Attempts:       rec.Attempts,
			ElapsedSeconds: output.Seconds(rec.Elapsed),
			Detail:         rec.Detail,
			LastError:      rec.LastError,
			Fatal:          rec.Fatal,
		})
	}
	return report
}

func splitHeader(raw string) (string, string, bool) {
	if key, value, ok := strings.Cut(raw, ":"); ok {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		return key, value, key != ""
	}
	if key, value, ok := strings.Cut(raw, "="); ok {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		return key, value, key != ""
	}
	return "", "", false
}

func indexOf(items []string, want string) int {
	for i, item := range items {
		if item == want {
			return i
		}
	}
	return -1
}

func helpText() string {
	return `waitfor - semantic condition poller

Usage:
  waitfor [flags] <backend> <target> [backend-flags]
  waitfor [flags] <backend> ... -- <backend> ...

Global flags:
  --timeout duration     Global deadline (default: 5m)
  --interval duration    Poll interval (default: 2s)
  --attempt-timeout duration
                         Per-attempt deadline (default: global remaining time)
  --output string        Output format: text|json (default: text)
  --mode string          Condition mode: all|any (default: all)
  --verbose              Show each attempt

HTTP:
  waitfor http URL [flags]
  --status 200|2xx          Expected status code or class
  --method GET              HTTP method
  --body text               Request body
  --body-file path          Request body from file
  --body-contains text      Required response substring
  --body-matches regex      Required response regex
  --jsonpath expr           Required JSON expression
  --header K=V              Request header
  --no-follow-redirects     Do not follow redirects

TCP:
  waitfor tcp HOST:PORT

Exec:
  waitfor exec [flags] -- COMMAND [ARGS...]
  --exit-code 0             Expected exit code
  --output-contains text    Required stdout/stderr substring
  --jsonpath expr           Required stdout JSON expression
  --cwd path                Working directory
  --env K=V                 Extra environment variable
  --max-output-bytes N      Capture at most N bytes

File:
  waitfor file PATH [exists|deleted|nonempty] [--contains text]

Kubernetes:
  waitfor k8s RESOURCE [--condition Ready] [--namespace default] [--jsonpath expr] [--kubeconfig path]

Examples:
  waitfor http https://api.example.com/health --status 200
  waitfor tcp localhost:5432
  waitfor file /tmp/ready.flag exists
  waitfor exec --output-contains Running -- kubectl get pod myapp
  waitfor --timeout 10m http https://api.example.com/health -- tcp localhost:5432

Exit codes:
  0    conditions satisfied
  1    timeout
  2    invalid arguments or configuration
  3    unrecoverable condition failure
  130  cancelled by SIGINT, SIGTERM, or parent context
`
}
