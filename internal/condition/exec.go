package condition

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/pbsladek/wait-for/internal/expr"
)

type ExecCondition struct {
	Command          []string
	ExpectedExitCode int
	OutputContains   string
	OutputJSONPath   string
	OutputJSONExpr   *expr.Expression
	Cwd              string
	Env              []string
	MaxOutputBytes   int64
}

func NewExec(command []string) *ExecCondition {
	return &ExecCondition{Command: command}
}

func (c *ExecCondition) Descriptor() Descriptor {
	target := strings.Join(c.Command, " ")
	return Descriptor{Backend: "exec", Target: target, Name: "exec " + target}
}

func (c *ExecCondition) Check(ctx context.Context) Result {
	if len(c.Command) == 0 {
		return Fatal(fmt.Errorf("exec command is required"))
	}

	cmd := exec.CommandContext(ctx, c.Command[0], c.Command[1:]...)
	cmd.Dir = c.Cwd
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	var output limitedBuffer
	output.limit = c.MaxOutputBytes
	writer := io.Writer(&output)
	cmd.Stdout = writer
	cmd.Stderr = writer
	err := cmd.Run()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return Fatal(err)
		}
	}

	if exitCode != c.ExpectedExitCode {
		detail := fmt.Sprintf("exit code %d, expected %d", exitCode, c.ExpectedExitCode)
		return Unsatisfied(detail, errors.New(detail))
	}

	out := output.Bytes()
	details := []string{fmt.Sprintf("exit code %d", exitCode)}
	if output.truncated {
		details = append(details, fmt.Sprintf("output truncated to %d bytes", c.MaxOutputBytes))
	}
	if c.OutputContains != "" {
		if !bytes.Contains(out, []byte(c.OutputContains)) {
			return Unsatisfied("output substring not found", fmt.Errorf("output does not contain %q", c.OutputContains))
		}
		details = append(details, fmt.Sprintf("output contains %q", c.OutputContains))
	}
	if c.OutputJSONPath != "" || c.OutputJSONExpr != nil {
		outputExpr := c.OutputJSONExpr
		if outputExpr == nil {
			var err error
			outputExpr, err = expr.Compile(c.OutputJSONPath)
			if err != nil {
				return Fatal(err)
			}
		}
		ok, detail, err := outputExpr.EvaluateJSON(out)
		if err != nil {
			return Fatal(err)
		}
		if !ok {
			return Unsatisfied(detail, fmt.Errorf("jsonpath condition not satisfied: %s", c.OutputJSONPath))
		}
		details = append(details, detail)
	}

	return Satisfied(strings.Join(details, ", "))
}

type limitedBuffer struct {
	bytes.Buffer
	limit     int64
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return b.Buffer.Write(p)
	}
	remaining := b.limit - int64(b.Buffer.Len())
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		b.truncated = true
		_, _ = b.Buffer.Write(p[:remaining])
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
