package condition

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPConditionSatisfied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Test"); got != "yes" {
			t.Fatalf("header = %q, want yes", got)
		}
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"ready":true,"message":"ok"}`)
	}))
	defer server.Close()

	cond := NewHTTP(server.URL)
	cond.ExpectedStatus = http.StatusAccepted
	cond.BodyContains = "ok"
	cond.BodyJSONPath = ".ready == true"
	cond.Headers["X-Test"] = "yes"

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("Satisfied = false, err = %v, detail = %q", result.Err, result.Detail)
	}
}

func TestHTTPConditionStatusRangeRequestBodyAndRegex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "service ready")
	}))
	defer server.Close()

	status, err := ParseHTTPStatusMatcher("2xx")
	if err != nil {
		t.Fatal(err)
	}
	cond := NewHTTP(server.URL)
	cond.Method = http.MethodPost
	cond.StatusMatcher = status
	cond.RequestBody = []byte("ping")
	cond.BodyMatches = `ready$`

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("Satisfied = false, err = %v, detail = %q", result.Err, result.Detail)
	}
}

func TestHTTPConditionNoRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ready", http.StatusFound)
	}))
	defer server.Close()

	status, err := ParseHTTPStatusMatcher("3xx")
	if err != nil {
		t.Fatal(err)
	}
	cond := NewHTTP(server.URL)
	cond.StatusMatcher = status
	cond.NoRedirects = true

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("Satisfied = false, err = %v, detail = %q", result.Err, result.Detail)
	}
}

func TestParseHTTPStatusMatcher(t *testing.T) {
	tests := []struct {
		raw  string
		code int
		want bool
	}{
		{raw: "200", code: 200, want: true},
		{raw: "2xx", code: 201, want: true},
		{raw: "2xx", code: 404, want: false},
	}

	for _, tt := range tests {
		matcher, err := ParseHTTPStatusMatcher(tt.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := matcher.Match(tt.code); got != tt.want {
			t.Fatalf("%s.Match(%d) = %v, want %v", tt.raw, tt.code, got, tt.want)
		}
	}
}

func TestHTTPConditionStatusMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	result := NewHTTP(server.URL).Check(t.Context())
	if result.Status == CheckSatisfied {
		t.Fatal("Satisfied = true, want false")
	}
	if result.Err == nil {
		t.Fatal("Err = nil, want status error")
	}
}
