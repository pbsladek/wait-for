package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

type Printer struct {
	w       io.Writer
	format  Format
	verbose bool
	mu      sync.Mutex
}

type Attempt struct {
	Name      string
	Attempt   int
	Satisfied bool
	Detail    string
	Error     string
	Elapsed   time.Duration
}

// Report is the stable output-facing run report serialized by JSON output.
type Report struct {
	Status                   string            `json:"status"`
	Satisfied                bool              `json:"satisfied"`
	Mode                     string            `json:"mode"`
	ElapsedSeconds           float64           `json:"elapsed_seconds"`
	TimeoutSeconds           float64           `json:"timeout_seconds"`
	IntervalSeconds          float64           `json:"interval_seconds"`
	PerAttemptTimeoutSeconds float64           `json:"per_attempt_timeout_seconds,omitempty"`
	Conditions               []ConditionReport `json:"conditions"`
}

type ConditionReport struct {
	Backend        string  `json:"backend,omitempty"`
	Target         string  `json:"target,omitempty"`
	Name           string  `json:"name"`
	Satisfied      bool    `json:"satisfied"`
	Attempts       int     `json:"attempts"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	Detail         string  `json:"detail,omitempty"`
	LastError      string  `json:"last_error,omitempty"`
	Fatal          bool    `json:"fatal,omitempty"`
}

func NewPrinter(w io.Writer, format Format, verbose bool) *Printer {
	return &Printer{w: w, format: format, verbose: verbose}
}

func (p *Printer) Start(count int, timeout time.Duration, interval time.Duration) {
	if p.format != FormatText {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.w, "[waitfor] checking %d condition(s) (timeout: %s, interval: %s)\n", count, timeout, interval)
}

func (p *Printer) Attempt(event Attempt) {
	if p.format != FormatText {
		return
	}
	if !p.verbose && !event.Satisfied {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if event.Satisfied {
		fmt.Fprintf(p.w, "[waitfor] [ok] %s (attempt %d, %.1fs) %s\n", event.Name, event.Attempt, event.Elapsed.Seconds(), event.Detail)
		return
	}
	if event.Error != "" {
		fmt.Fprintf(p.w, "[waitfor] [..] %s (attempt %d) %s\n", event.Name, event.Attempt, event.Error)
		return
	}
	fmt.Fprintf(p.w, "[waitfor] [..] %s (attempt %d) %s\n", event.Name, event.Attempt, event.Detail)
}

func (p *Printer) Outcome(report Report) error {
	if p.format == FormatJSON {
		return WriteJSON(p.w, report)
	}
	switch report.Status {
	case "satisfied":
		fmt.Fprintf(p.w, "[waitfor] conditions satisfied in %.3fs\n", report.ElapsedSeconds)
		return nil
	case "fatal":
		fmt.Fprintf(p.w, "[waitfor] stopped after %.3fs due to unrecoverable error\n", report.ElapsedSeconds)
	case "cancelled":
		fmt.Fprintf(p.w, "[waitfor] cancelled after %.3fs\n", report.ElapsedSeconds)
	default:
		fmt.Fprintf(p.w, "[waitfor] timeout after %.3fs\n", report.ElapsedSeconds)
	}
	for _, rec := range report.Conditions {
		if rec.Satisfied {
			continue
		}
		if rec.LastError != "" {
			fmt.Fprintf(p.w, "[waitfor] unsatisfied: %s: %s\n", rec.Name, rec.LastError)
		} else {
			fmt.Fprintf(p.w, "[waitfor] unsatisfied: %s\n", rec.Name)
		}
	}
	return nil
}

func WriteJSON(w io.Writer, report Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func roundSeconds(v float64) float64 {
	return float64(int(v*1000+0.5)) / 1000
}

func Seconds(d time.Duration) float64 {
	return roundSeconds(d.Seconds())
}
