package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pbsladek/wait-for/internal/condition"
	"github.com/pbsladek/wait-for/internal/output"
	"github.com/pbsladek/wait-for/internal/runner"
)

type explainReport struct {
	TimeoutSeconds           float64                  `json:"timeout_seconds"`
	IntervalSeconds          float64                  `json:"interval_seconds"`
	MaxIntervalSeconds       float64                  `json:"max_interval_seconds"`
	Backoff                  string                   `json:"backoff"`
	Jitter                   float64                  `json:"jitter"`
	PerAttemptTimeoutSeconds float64                  `json:"per_attempt_timeout_seconds,omitempty"`
	RequiredSuccesses        int                      `json:"required_successes"`
	StableForSeconds         float64                  `json:"stable_for_seconds,omitempty"`
	Mode                     string                   `json:"mode"`
	Profile                  string                   `json:"profile,omitempty"`
	ConfigFile               string                   `json:"config_file,omitempty"`
	Conditions               []explainConditionReport `json:"conditions"`
}

type explainConditionReport struct {
	Backend string `json:"backend"`
	Target  string `json:"target"`
	Name    string `json:"name"`
	Guard   bool   `json:"guard,omitempty"`
}

func writeExplain(stdout io.Writer, _ io.Writer, opts globalOptions, cfg runner.Config) (int, error) {
	report := buildExplainReport(opts, cfg)
	switch opts.format {
	case output.FormatJSON:
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return ExitSatisfied, enc.Encode(report)
	case output.FormatNDJSON:
		enc := json.NewEncoder(stdout)
		if err := enc.Encode(map[string]any{"event": "explain", "plan": report}); err != nil {
			return ExitFatal, err
		}
		return ExitSatisfied, nil
	default:
		writeExplainText(stdout, report)
		return ExitSatisfied, nil
	}
}

func buildExplainReport(opts globalOptions, cfg runner.Config) explainReport {
	report := explainReport{
		TimeoutSeconds:           output.Seconds(cfg.Timeout),
		IntervalSeconds:          output.Seconds(cfg.Interval),
		MaxIntervalSeconds:       output.Seconds(cfg.MaxInterval),
		Backoff:                  string(cfg.Backoff),
		Jitter:                   cfg.Jitter,
		PerAttemptTimeoutSeconds: output.Seconds(cfg.PerAttemptTimeout),
		RequiredSuccesses:        cfg.RequiredSuccesses,
		StableForSeconds:         output.Seconds(cfg.StableFor),
		Mode:                     string(cfg.Mode),
		Profile:                  opts.profile,
		ConfigFile:               opts.configFile,
		Conditions:               make([]explainConditionReport, 0, len(cfg.Conditions)),
	}
	for _, cond := range cfg.Conditions {
		desc := cond.Descriptor()
		report.Conditions = append(report.Conditions, explainConditionReport{
			Backend: desc.Backend,
			Target:  desc.Target,
			Name:    desc.DisplayName(),
			Guard:   conditionRoleForExplain(cond) == condition.RoleGuard,
		})
	}
	return report
}

func conditionRoleForExplain(cond condition.Condition) condition.Role {
	if provider, ok := cond.(condition.RoleProvider); ok {
		return provider.ConditionRole()
	}
	if wrapper, ok := cond.(condition.Wrapper); ok {
		return conditionRoleForExplain(wrapper.UnwrapCondition())
	}
	return condition.RoleReady
}

func writeExplainText(w io.Writer, report explainReport) {
	_, _ = fmt.Fprintf(w, "waitfor plan: %d condition(s)\n", len(report.Conditions))
	_, _ = fmt.Fprintf(w, "mode: %s\n", report.Mode)
	_, _ = fmt.Fprintf(w, "timeout: %.3fs\n", report.TimeoutSeconds)
	_, _ = fmt.Fprintf(w, "interval: %.3fs\n", report.IntervalSeconds)
	_, _ = fmt.Fprintf(w, "backoff: %s", report.Backoff)
	if report.Jitter > 0 {
		_, _ = fmt.Fprintf(w, " jitter=%.3f", report.Jitter)
	}
	_, _ = fmt.Fprintln(w)
	if report.Profile != "" {
		_, _ = fmt.Fprintf(w, "profile: %s\n", report.Profile)
	}
	if report.ConfigFile != "" {
		_, _ = fmt.Fprintf(w, "config: %s\n", report.ConfigFile)
	}
	for _, cond := range report.Conditions {
		role := "ready"
		if cond.Guard {
			role = "guard"
		}
		_, _ = fmt.Fprintf(w, "- [%s] %s %s name=%q\n", role, cond.Backend, cond.Target, cond.Name)
	}
}
