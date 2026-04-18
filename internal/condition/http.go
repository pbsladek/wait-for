package condition

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pbsladek/wait-for/internal/expr"
)

type HTTPCondition struct {
	URL            string
	Method         string
	ExpectedStatus int
	StatusMatcher  HTTPStatusMatcher
	RequestBody    []byte
	BodyContains   string
	BodyMatches    string
	BodyRegex      *regexp.Regexp
	BodyJSONPath   string
	BodyJSONExpr   *expr.Expression
	Insecure       bool
	NoRedirects    bool
	Headers        map[string]string
	Client         *http.Client
	clientOnce     sync.Once
	clientCache    *http.Client
}

type HTTPStatusMatcher struct {
	raw   string
	exact int
	class int
}

func ParseHTTPStatusMatcher(raw string) (HTTPStatusMatcher, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "200"
	}
	if len(raw) == 3 && raw[1:] == "xx" && raw[0] >= '1' && raw[0] <= '5' {
		return HTTPStatusMatcher{raw: raw, class: int(raw[0] - '0')}, nil
	}
	code, err := strconv.Atoi(raw)
	if err != nil || code < 100 || code > 599 {
		return HTTPStatusMatcher{}, fmt.Errorf("invalid HTTP status %q", raw)
	}
	return HTTPStatusMatcher{raw: raw, exact: code}, nil
}

func (m HTTPStatusMatcher) Match(code int) bool {
	if m.class != 0 {
		return code/100 == m.class
	}
	expected := m.exact
	if expected == 0 {
		expected = http.StatusOK
	}
	return code == expected
}

func (m HTTPStatusMatcher) String() string {
	if m.raw != "" {
		return m.raw
	}
	if m.exact != 0 {
		return strconv.Itoa(m.exact)
	}
	return "200"
}

func NewHTTP(url string) *HTTPCondition {
	return &HTTPCondition{
		URL:            url,
		Method:         http.MethodGet,
		ExpectedStatus: http.StatusOK,
		Headers:        map[string]string{},
	}
}

func (c *HTTPCondition) Descriptor() Descriptor {
	return Descriptor{Backend: "http", Target: c.URL, Name: fmt.Sprintf("http %s", c.URL)}
}

func (c *HTTPCondition) Check(ctx context.Context) Result {
	method := c.Method
	if method == "" {
		method = http.MethodGet
	}
	statusMatcher := c.statusMatcher()

	req, err := http.NewRequestWithContext(ctx, method, c.URL, bytes.NewReader(c.RequestBody))
	if err != nil {
		return Fatal(err)
	}
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}

	resp, err := c.client().Do(req)
	if err != nil {
		return Unsatisfied("", err)
	}
	defer resp.Body.Close()

	bodyNeeded := c.BodyContains != "" || c.BodyMatches != "" || c.BodyRegex != nil || c.BodyJSONPath != "" || c.BodyJSONExpr != nil || !statusMatcher.Match(resp.StatusCode)
	var body []byte
	if bodyNeeded {
		body, err = io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
		if err != nil {
			return Unsatisfied("", err)
		}
	}

	if !statusMatcher.Match(resp.StatusCode) {
		detail := fmt.Sprintf("status %d, expected %s", resp.StatusCode, statusMatcher.String())
		if trimmed := strings.TrimSpace(string(body)); trimmed != "" {
			detail = fmt.Sprintf("%s: %s", detail, firstLine(trimmed))
		}
		return Unsatisfied(detail, errors.New(detail))
	}

	details := []string{fmt.Sprintf("status %d", resp.StatusCode)}
	if c.BodyContains != "" {
		if !bytes.Contains(body, []byte(c.BodyContains)) {
			return Unsatisfied("body substring not found", fmt.Errorf("body does not contain %q", c.BodyContains))
		}
		details = append(details, fmt.Sprintf("body contains %q", c.BodyContains))
	}
	bodyRegex := c.BodyRegex
	if bodyRegex == nil && c.BodyMatches != "" {
		var err error
		bodyRegex, err = regexp.Compile(c.BodyMatches)
		if err != nil {
			return Fatal(err)
		}
	}
	if bodyRegex != nil {
		if !bodyRegex.Match(body) {
			return Unsatisfied("body regex not matched", fmt.Errorf("body does not match %q", bodyRegex.String()))
		}
		details = append(details, fmt.Sprintf("body matches %q", bodyRegex.String()))
	}
	bodyExpr := c.BodyJSONExpr
	if bodyExpr == nil && c.BodyJSONPath != "" {
		var err error
		bodyExpr, err = expr.Compile(c.BodyJSONPath)
		if err != nil {
			return Fatal(err)
		}
	}
	if bodyExpr != nil {
		ok, detail, err := bodyExpr.EvaluateJSON(body)
		if err != nil {
			return Fatal(err)
		}
		if !ok {
			return Unsatisfied(detail, fmt.Errorf("jsonpath condition not satisfied: %s", c.BodyJSONPath))
		}
		details = append(details, detail)
	}

	return Satisfied(strings.Join(details, ", "))
}

func (c *HTTPCondition) statusMatcher() HTTPStatusMatcher {
	if c.StatusMatcher.raw != "" || c.StatusMatcher.exact != 0 || c.StatusMatcher.class != 0 {
		return c.StatusMatcher
	}
	if c.ExpectedStatus != 0 {
		return HTTPStatusMatcher{raw: strconv.Itoa(c.ExpectedStatus), exact: c.ExpectedStatus}
	}
	status, _ := ParseHTTPStatusMatcher("200")
	return status
}

func (c *HTTPCondition) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	if !c.Insecure && !c.NoRedirects {
		return http.DefaultClient
	}
	c.clientOnce.Do(func() {
		transport := http.DefaultTransport
		if c.Insecure {
			transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
		}
		c.clientCache = &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		}
		if c.NoRedirects {
			c.clientCache.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
		}
	})
	return c.clientCache
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	if len(line) > 200 {
		return line[:200]
	}
	return line
}
