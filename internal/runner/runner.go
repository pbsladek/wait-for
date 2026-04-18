package runner

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/pbsladek/wait-for/internal/condition"
	"golang.org/x/sync/errgroup"
)

type Mode int

const (
	ModeAll Mode = iota
	ModeAny
)

func (m Mode) String() string {
	if m == ModeAny {
		return "any"
	}
	return "all"
}

type Status string

const (
	StatusSatisfied Status = "satisfied"
	StatusTimeout   Status = "timeout"
	StatusCancelled Status = "cancelled"
	StatusFatal     Status = "fatal"
)

type Config struct {
	Conditions        []condition.Condition
	Timeout           time.Duration
	Interval          time.Duration
	PerAttemptTimeout time.Duration
	Mode              Mode
	OnAttempt         func(AttemptEvent)
}

type AttemptEvent struct {
	Name      string
	Attempt   int
	Satisfied bool
	Detail    string
	Error     string
	Elapsed   time.Duration
}

type ConditionResult struct {
	Backend   string
	Target    string
	Name      string
	Satisfied bool
	Attempts  int
	Elapsed   time.Duration
	Detail    string
	LastError string
	Fatal     bool
}

type Outcome struct {
	Status            Status
	Mode              Mode
	Elapsed           time.Duration
	Timeout           time.Duration
	Interval          time.Duration
	PerAttemptTimeout time.Duration
	Conditions        []ConditionResult
}

func (o Outcome) Satisfied() bool {
	return o.Status == StatusSatisfied
}

func (o Outcome) TimedOut() bool {
	return o.Status == StatusTimeout
}

func (o Outcome) Cancelled() bool {
	return o.Status == StatusCancelled
}

func (o Outcome) Fatal() bool {
	return o.Status == StatusFatal
}

func Run(ctx context.Context, cfg Config) (Outcome, error) {
	if len(cfg.Conditions) == 0 {
		return Outcome{}, errors.New("at least one condition is required")
	}
	if cfg.Timeout <= 0 {
		return Outcome{}, errors.New("timeout must be positive")
	}
	if cfg.Interval <= 0 {
		return Outcome{}, errors.New("interval must be positive")
	}
	if cfg.PerAttemptTimeout < 0 {
		return Outcome{}, errors.New("per-attempt timeout cannot be negative")
	}
	if cfg.PerAttemptTimeout > cfg.Timeout {
		cfg.PerAttemptTimeout = cfg.Timeout
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	records := make([]ConditionResult, len(cfg.Conditions))
	for i, cond := range cfg.Conditions {
		desc := cond.Descriptor()
		records[i].Backend = desc.Backend
		records[i].Target = desc.Target
		records[i].Name = desc.DisplayName()
	}

	var mu sync.Mutex
	g, runCtx := errgroup.WithContext(ctx)

	for i, cond := range cfg.Conditions {
		i := i
		cond := cond
		g.Go(func() error {
			runCondition(runCtx, cond, cfg, start, &records[i], &mu, cancel)
			return nil
		})
	}

	_ = g.Wait()

	mu.Lock()
	out := Outcome{
		Mode:              cfg.Mode,
		Elapsed:           time.Since(start),
		Timeout:           cfg.Timeout,
		Interval:          cfg.Interval,
		PerAttemptTimeout: cfg.PerAttemptTimeout,
		Conditions:        append([]ConditionResult(nil), records...),
	}
	mu.Unlock()

	fatal := false
	for _, rec := range out.Conditions {
		if rec.Fatal {
			fatal = true
			break
		}
	}
	if fatal {
		out.Status = StatusFatal
		return out, nil
	}
	if outcomeSatisfied(out.Conditions, cfg.Mode) {
		out.Status = StatusSatisfied
		return out, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		out.Status = StatusTimeout
		return out, nil
	}
	out.Status = StatusCancelled
	return out, nil
}

func runCondition(
	ctx context.Context,
	cond condition.Condition,
	cfg Config,
	start time.Time,
	record *ConditionResult,
	mu *sync.Mutex,
	cancel context.CancelFunc,
) {
	conditionStart := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		attempt := 0
		mu.Lock()
		record.Attempts++
		attempt = record.Attempts
		mu.Unlock()

		attemptCtx := ctx
		attemptCancel := func() {}
		if cfg.PerAttemptTimeout > 0 {
			attemptCtx, attemptCancel = context.WithTimeout(ctx, cfg.PerAttemptTimeout)
		}
		result := cond.Check(attemptCtx)
		attemptCancel()
		checkSatisfied := result.Status == condition.CheckSatisfied
		checkFatal := result.Status == condition.CheckFatal
		event := AttemptEvent{
			Name:      record.Name,
			Attempt:   attempt,
			Satisfied: checkSatisfied,
			Detail:    result.Detail,
			Elapsed:   time.Since(start),
		}
		if result.Err != nil {
			event.Error = result.Err.Error()
		}

		mu.Lock()
		record.Elapsed = time.Since(conditionStart)
		record.Detail = result.Detail
		if result.Err != nil {
			record.LastError = result.Err.Error()
		}
		if checkFatal {
			record.Fatal = true
		}
		if checkSatisfied {
			record.Satisfied = true
		}
		mu.Unlock()

		if cfg.OnAttempt != nil {
			cfg.OnAttempt(event)
		}

		if checkFatal {
			cancel()
			return
		}
		if checkSatisfied {
			if cfg.Mode == ModeAny {
				cancel()
			}
			return
		}

		timer := time.NewTimer(cfg.Interval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
}

func outcomeSatisfied(records []ConditionResult, mode Mode) bool {
	if mode == ModeAny {
		for _, rec := range records {
			if rec.Satisfied {
				return true
			}
		}
		return false
	}
	for _, rec := range records {
		if !rec.Satisfied {
			return false
		}
	}
	return true
}
