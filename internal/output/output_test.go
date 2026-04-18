package output

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	err := WriteJSON(&buf, Report{
		Status:          "satisfied",
		Satisfied:       true,
		Mode:            "all",
		ElapsedSeconds:  1.5,
		TimeoutSeconds:  60,
		IntervalSeconds: 1,
		Conditions: []ConditionReport{
			{Backend: "tcp", Target: "localhost:5432", Name: "tcp localhost:5432", Satisfied: true, Attempts: 2, ElapsedSeconds: 1.4, Detail: "connection established"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "status": "satisfied",
  "satisfied": true,
  "mode": "all",
  "elapsed_seconds": 1.5,
  "timeout_seconds": 60,
  "interval_seconds": 1,
  "conditions": [
    {
      "backend": "tcp",
      "target": "localhost:5432",
      "name": "tcp localhost:5432",
      "satisfied": true,
      "attempts": 2,
      "elapsed_seconds": 1.4,
      "detail": "connection established"
    }
  ]
}
`
	if got := buf.String(); got != want {
		t.Fatalf("json output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteJSONStatuses(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "timeout", status: "timeout", want: `"status": "timeout"`},
		{name: "cancelled", status: "cancelled", want: `"status": "cancelled"`},
		{name: "fatal", status: "fatal", want: `"status": "fatal"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteJSON(&buf, Report{
				Status:          tt.status,
				Mode:            "any",
				ElapsedSeconds:  2,
				TimeoutSeconds:  5,
				IntervalSeconds: 1,
				Conditions: []ConditionReport{
					{Name: "condition", Attempts: 1, ElapsedSeconds: 1, LastError: "not ready", Fatal: tt.status == "fatal"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(buf.String(), tt.want) {
				t.Fatalf("output %q does not contain %q", buf.String(), tt.want)
			}
			if !strings.Contains(buf.String(), `"mode": "any"`) {
				t.Fatalf("output %q does not contain mode", buf.String())
			}
		})
	}
}

func TestPrinterText(t *testing.T) {
	var buf bytes.Buffer
	printer := NewPrinter(&buf, FormatText, true)
	printer.Start(1, time.Second, time.Millisecond)
	printer.Attempt(Attempt{Name: "file /tmp/ready exists", Attempt: 1, Satisfied: true, Detail: "exists"})
	if err := printer.Outcome(Report{Status: "satisfied", Satisfied: true}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"checking 1 condition", "[ok] file /tmp/ready exists", "conditions satisfied"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
}
