package condition

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"testing"

	wdns "codeberg.org/miekg/dns"
)

func TestValidateDNSName(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{name: "plain", host: "example.test"},
		{name: "root", host: "."},
		{name: "service labels", host: "_sip._tcp.example.test"},
		{name: "leading whitespace", host: " example.test", wantErr: true},
		{name: "embedded whitespace", host: "bad name", wantErr: true},
		{name: "control", host: "bad\tname", wantErr: true},
		{name: "empty label", host: "bad..name", wantErr: true},
		{name: "leading hyphen", host: "-bad.example.test", wantErr: true},
		{name: "trailing hyphen", host: "bad-.example.test", wantErr: true},
		{name: "invalid character", host: "bad/name", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDNSName(tt.host)
			if tt.wantErr && err == nil {
				t.Fatal("ValidateDNSName() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateDNSName() error = %v", err)
			}
		})
	}
}

func TestNormalizeDNSServer(t *testing.T) {
	tests := []struct {
		name    string
		server  string
		want    string
		wantErr bool
	}{
		{name: "empty"},
		{name: "host without port", server: "dns.example.test", want: "dns.example.test:53"},
		{name: "ipv4 without port", server: "192.0.2.53", want: "192.0.2.53:53"},
		{name: "ipv6 without port", server: "2001:db8::53", want: "[2001:db8::53]:53"},
		{name: "bracketed ipv6 without port", server: "[2001:db8::53]", want: "[2001:db8::53]:53"},
		{name: "with port", server: "dns.example.test:5353", want: "dns.example.test:5353"},
		{name: "blank port", server: "dns.example.test:", wantErr: true},
		{name: "bad port", server: "dns.example.test:0", wantErr: true},
		{name: "whitespace", server: " dns.example.test", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeDNSServer(tt.server)
			if tt.wantErr && err == nil {
				t.Fatal("NormalizeDNSServer() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("NormalizeDNSServer() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeDNSServer() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDNSConditionARecordSatisfied(t *testing.T) {
	cond := NewDNS("example.test")
	cond.LookupIP = func(_ context.Context, network, host string) ([]net.IP, error) {
		if network != "ip4" || host != "example.test" {
			t.Fatalf("LookupIP(%q, %q), want ip4 example.test", network, host)
		}
		return []net.IP{net.ParseIP("192.0.2.10")}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionAAAARecordSatisfied(t *testing.T) {
	cond := NewDNS("example.test")
	cond.RecordType = DNSRecordAAAA
	cond.LookupIP = func(_ context.Context, network, _ string) ([]net.IP, error) {
		if network != "ip6" {
			t.Fatalf("network = %q, want ip6", network)
		}
		return []net.IP{net.ParseIP("2001:db8::1")}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionCNAMEContains(t *testing.T) {
	cond := NewDNS("app.example.test")
	cond.RecordType = DNSRecordCNAME
	cond.Contains = "target"
	cond.LookupCNAME = func(_ context.Context, _ string) (string, error) {
		return "target.example.test.", nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionTXTContainsMissing(t *testing.T) {
	cond := NewDNS("example.test")
	cond.RecordType = DNSRecordTXT
	cond.Contains = "ready"
	cond.LookupTXT = func(_ context.Context, _ string) ([]string, error) {
		return []string{"not-yet"}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckUnsatisfied {
		t.Fatalf("status = %s, want unsatisfied", result.Status)
	}
}

func TestDNSConditionAnyUsesLookupHost(t *testing.T) {
	cond := NewDNS("example.test")
	cond.RecordType = DNSRecordANY
	cond.LookupHost = func(_ context.Context, _ string) ([]string, error) {
		return []string{"192.0.2.10"}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionLookupErrorUnsatisfied(t *testing.T) {
	cond := NewDNS("missing.example.test")
	cond.LookupIP = func(_ context.Context, _, _ string) ([]net.IP, error) {
		return nil, fmt.Errorf("no such host")
	}

	result := cond.Check(t.Context())
	if result.Status != CheckUnsatisfied {
		t.Fatalf("status = %s, want unsatisfied", result.Status)
	}
}

func TestDNSConditionInvalidRecordType(t *testing.T) {
	cond := NewDNS("example.test")
	cond.RecordType = "MX"

	result := cond.Check(t.Context())
	if result.Status != CheckFatal {
		t.Fatalf("status = %s, want fatal", result.Status)
	}
}

func TestDNSConditionWireRequiresServerWithoutInjectedExchange(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire

	result := cond.Check(t.Context())
	if result.Status != CheckFatal {
		t.Fatalf("status = %s, want fatal", result.Status)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "requires a server") {
		t.Fatalf("err = %v, want server validation error", result.Err)
	}
}

func TestDNSConditionInvalidMatcherConfigFatal(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*DNSCondition)
	}{
		{"negative min count", func(c *DNSCondition) { c.MinCount = -1 }},
		{"absent with contains", func(c *DNSCondition) {
			c.Absent = true
			c.Contains = "ready"
		}},
		{"absent with equals", func(c *DNSCondition) {
			c.Absent = true
			c.Equals = []string{"192.0.2.10"}
		}},
		{"absent with min count", func(c *DNSCondition) {
			c.Absent = true
			c.MinCount = 1
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewDNS("example.test")
			cond.LookupIP = func(context.Context, string, string) ([]net.IP, error) {
				t.Fatal("LookupIP should not be called for invalid matcher config")
				return nil, nil
			}
			tt.setup(cond)

			result := cond.Check(t.Context())
			if result.Status != CheckFatal {
				t.Fatalf("status = %s, want fatal", result.Status)
			}
		})
	}
}

func TestDNSConditionInvalidRCodeFatal(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.RCode = "READY"
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		t.Fatal("WireExchange should not be called for invalid rcode")
		return nil, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckFatal {
		t.Fatalf("status = %s, want fatal", result.Status)
	}
}

func TestDNSConditionInvalidResolverModeFatal(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = "raw"

	result := cond.Check(t.Context())
	if result.Status != CheckFatal {
		t.Fatalf("status = %s, want fatal", result.Status)
	}
}

func TestDNSConditionWireOnlyOptionsFatalWithSystemResolver(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*DNSCondition)
	}{
		{"rcode", func(c *DNSCondition) { c.RCode = "NOERROR" }},
		{"absent mode", func(c *DNSCondition) { c.AbsentMode = DNSAbsentNXDomain }},
		{"wire record type", func(c *DNSCondition) { c.RecordType = DNSRecordMX }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewDNS("example.test")
			tt.setup(cond)

			result := cond.Check(t.Context())
			if result.Status != CheckFatal {
				t.Fatalf("status = %s, want fatal", result.Status)
			}
		})
	}
}

func TestDNSConditionInvalidAbsentModeAndTransportFatal(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*DNSCondition)
	}{
		{"absent mode", func(c *DNSCondition) { c.AbsentMode = "gone" }},
		{"transport", func(c *DNSCondition) { c.Transport = "quic" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewDNS("example.test")
			cond.ResolverMode = DNSResolverWire
			tt.setup(cond)

			result := cond.Check(t.Context())
			if result.Status != CheckFatal {
				t.Fatalf("status = %s, want fatal", result.Status)
			}
		})
	}
}

func TestDNSConditionInvalidUDPSizeFatal(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		t.Fatal("WireExchange should not be called for invalid UDP size")
		return nil, nil
	}
	cond.UDPSize = 128
	result := cond.Check(t.Context())
	if result.Status != CheckFatal {
		t.Fatalf("status = %s, want fatal", result.Status)
	}
}

func TestDNSConditionEmptyHostFatal(t *testing.T) {
	result := NewDNS(" ").Check(t.Context())
	if result.Status != CheckFatal {
		t.Fatalf("status = %s, want fatal", result.Status)
	}
}

func TestDNSConditionEquals(t *testing.T) {
	cond := NewDNS("example.test")
	cond.Equals = []string{"192.0.2.10"}
	cond.LookupIP = func(_ context.Context, _, _ string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.0.2.10")}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionEqualsMissing(t *testing.T) {
	cond := NewDNS("example.test")
	cond.Equals = []string{"192.0.2.20"}
	cond.LookupIP = func(_ context.Context, _, _ string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.0.2.10")}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckUnsatisfied {
		t.Fatalf("status = %s, want unsatisfied", result.Status)
	}
}

func TestDNSConditionCNAMEEqualsNormalizesCaseAndTrailingDot(t *testing.T) {
	cond := NewDNS("app.example.test")
	cond.RecordType = DNSRecordCNAME
	cond.Equals = []string{"target.example.test"}
	cond.LookupCNAME = func(_ context.Context, _ string) (string, error) {
		return "Target.Example.Test.", nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionMinCount(t *testing.T) {
	cond := NewDNS("example.test")
	cond.MinCount = 2
	cond.LookupIP = func(_ context.Context, _, _ string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.0.2.10")}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckUnsatisfied {
		t.Fatalf("status = %s, want unsatisfied", result.Status)
	}
}

func TestDNSConditionAbsentSatisfiedByNotFound(t *testing.T) {
	cond := NewDNS("missing.example.test")
	cond.Absent = true
	cond.LookupIP = func(_ context.Context, _, _ string) ([]net.IP, error) {
		return nil, &net.DNSError{IsNotFound: true}
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionAbsentUnsatisfiedWhenFound(t *testing.T) {
	cond := NewDNS("example.test")
	cond.Absent = true
	cond.LookupIP = func(_ context.Context, _, _ string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.0.2.10")}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckUnsatisfied {
		t.Fatalf("status = %s, want unsatisfied", result.Status)
	}
}

func TestDNSConditionInvalidNameFatal(t *testing.T) {
	longLabel := strings.Repeat("a", 64) + ".example.test"
	result := NewDNS(longLabel).Check(t.Context())
	if result.Status != CheckFatal {
		t.Fatalf("status = %s, want fatal", result.Status)
	}
}

func TestDNSConditionWireARecordSatisfied(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.Server = "127.0.0.1:53"
	cond.Equals = []string{"192.0.2.10"}
	cond.WireExchange = func(_ context.Context, _ *wdns.Msg, network, server string) (*wdns.Msg, error) {
		if network != "udp" || server != "127.0.0.1:53" {
			t.Fatalf("exchange network/server = %s/%s", network, server)
		}
		return wireResponse(wdns.RcodeSuccess, mustWireRR(t, "example.test. 60 IN A 192.0.2.10")), nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionWireUsesFQDNQuestion(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.Server = "127.0.0.1:53"
	cond.WireExchange = func(_ context.Context, msg *wdns.Msg, _, _ string) (*wdns.Msg, error) {
		if len(msg.Question) != 1 || msg.Question[0].Header().Name != "example.test." {
			t.Fatalf("question = %+v, want FQDN", msg.Question)
		}
		return wireResponse(wdns.RcodeSuccess, mustWireRR(t, "example.test. 60 IN A 192.0.2.10")), nil
	}
	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionWireNormalizesDirectServer(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.Server = "192.0.2.53"
	cond.WireExchange = func(_ context.Context, _ *wdns.Msg, _ string, server string) (*wdns.Msg, error) {
		if server != "192.0.2.53:53" {
			t.Fatalf("server = %q, want default port", server)
		}
		return wireResponse(wdns.RcodeSuccess, mustWireRR(t, "example.test. 60 IN A 192.0.2.10")), nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionInvalidDirectServerFatalBeforeExchange(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.Server = "192.0.2.53:0"
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		t.Fatal("WireExchange should not be called for invalid server")
		return nil, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckFatal {
		t.Fatalf("status = %s, want fatal", result.Status)
	}
}

func TestDNSConditionWireUDPSizeDefaultsOnlyWhenEDNS0Enabled(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*DNSCondition)
		wantUDP uint16
	}{
		{name: "default", wantUDP: 0},
		{name: "edns0 default size", setup: func(c *DNSCondition) {
			c.EDNS0 = true
		}, wantUDP: wdns.DefaultMsgSize},
		{name: "explicit size", setup: func(c *DNSCondition) {
			c.UDPSize = 1232
		}, wantUDP: 1232},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewDNS("example.test")
			cond.ResolverMode = DNSResolverWire
			cond.Server = "192.0.2.53"
			if tt.setup != nil {
				tt.setup(cond)
			}
			cond.WireExchange = func(_ context.Context, msg *wdns.Msg, _ string, _ string) (*wdns.Msg, error) {
				if msg.UDPSize != tt.wantUDP {
					t.Fatalf("UDPSize = %d, want %d", msg.UDPSize, tt.wantUDP)
				}
				return wireResponse(wdns.RcodeSuccess, mustWireRR(t, "example.test. 60 IN A 192.0.2.10")), nil
			}

			result := cond.Check(t.Context())
			if result.Status != CheckSatisfied {
				t.Fatalf("status = %s, err = %v", result.Status, result.Err)
			}
		})
	}
}

func TestDNSConditionWireNXDomainAbsentMode(t *testing.T) {
	cond := NewDNS("missing.example.test")
	cond.ResolverMode = DNSResolverWire
	cond.Absent = true
	cond.AbsentMode = DNSAbsentNXDomain
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		return wireResponseFor("missing.example.test.", wdns.TypeA, wdns.RcodeNameError), nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionWireNODATAAbsentMode(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.Absent = true
	cond.AbsentMode = DNSAbsentNODATA
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		return wireResponse(wdns.RcodeSuccess), nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDNSConditionWireRCodeMismatch(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.RCode = "NXDOMAIN"
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		return wireResponse(wdns.RcodeSuccess, mustWireRR(t, "example.test. 60 IN A 192.0.2.10")), nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckUnsatisfied {
		t.Fatalf("status = %s, want unsatisfied", result.Status)
	}
}

func TestDNSConditionWireRCodeOnlySatisfied(t *testing.T) {
	tests := []struct {
		name  string
		rcode uint16
		want  string
	}{
		{"servfail", wdns.RcodeServerFailure, "SERVFAIL"},
		{"refused", wdns.RcodeRefused, "REFUSED"},
		{"nxdomain", wdns.RcodeNameError, "NXDOMAIN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewDNS("example.test")
			cond.ResolverMode = DNSResolverWire
			cond.RCode = " " + tt.want + " "
			cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
				return wireResponse(tt.rcode), nil
			}

			result := cond.Check(t.Context())
			if result.Status != CheckSatisfied {
				t.Fatalf("status = %s, err = %v, want satisfied", result.Status, result.Err)
			}
			if result.Detail != "rcode "+tt.want {
				t.Fatalf("detail = %q, want rcode %s", result.Detail, tt.want)
			}
		})
	}
}

func TestDNSConditionWireEmptyResponseUnsatisfied(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		return nil, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckUnsatisfied {
		t.Fatalf("status = %s, want unsatisfied", result.Status)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "empty dns response") {
		t.Fatalf("err = %v, want empty dns response", result.Err)
	}
}

func TestDNSConditionWireFiltersAnswersByRequestedType(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.RecordType = DNSRecordA
	cond.Equals = []string{"192.0.2.10"}
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		return wireResponse(
			wdns.RcodeSuccess,
			mustWireRR(t, "example.test. 60 IN CNAME target.example.test."),
			mustWireRR(t, "example.test. 60 IN A 192.0.2.10"),
			mustWireRR(t, "example.test. 60 IN MX 10 mail.example.test."),
		), nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v, want satisfied", result.Status, result.Err)
	}

	response, err := cond.lookup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(response.Values, ","); got != "192.0.2.10" {
		t.Fatalf("values = %q, want only A answer", got)
	}
}

func TestDNSConditionWireNODATAIgnoresOtherAnswerTypes(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.RecordType = DNSRecordA
	cond.Absent = true
	cond.AbsentMode = DNSAbsentNODATA
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		return wireResponse(wdns.RcodeSuccess, mustWireRR(t, "example.test. 60 IN MX 10 mail.example.test.")), nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v, want satisfied", result.Status, result.Err)
	}
}

func TestDNSConditionWireRetriesTruncatedUDPOverTCP(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	var networks []string
	cond.WireExchange = func(_ context.Context, _ *wdns.Msg, network, _ string) (*wdns.Msg, error) {
		networks = append(networks, network)
		if network == "udp" {
			response := wireResponse(wdns.RcodeSuccess)
			response.Truncated = true
			return response, nil
		}
		return wireResponse(wdns.RcodeSuccess, mustWireRR(t, "example.test. 60 IN A 192.0.2.10")), nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
	if strings.Join(networks, ",") != "udp,tcp" {
		t.Fatalf("networks = %v, want udp,tcp", networks)
	}
}

func TestDNSValueFromRRFormatsSupportedTypes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"a", "example.test. 60 IN A 192.0.2.10", "192.0.2.10"},
		{"aaaa", "example.test. 60 IN AAAA 2001:db8::1", "2001:db8::1"},
		{"cname", "example.test. 60 IN CNAME target.example.test.", "target.example.test."},
		{"txt", `example.test. 60 IN TXT "one" "two"`, "onetwo"},
		{"mx", "example.test. 60 IN MX 10 mail.example.test.", "10 mail.example.test."},
		{"srv", "_api._tcp.example.test. 60 IN SRV 1 2 443 target.example.test.", "1 2 443 target.example.test."},
		{"ns", "example.test. 60 IN NS ns1.example.test.", "ns1.example.test."},
		{"caa", `example.test. 60 IN CAA 0 issue "letsencrypt.org"`, "0 issue letsencrypt.org"},
		{"https", "example.test. 60 IN HTTPS 1 svc.example.test. alpn=h2", `1 svc.example.test. alpn="h2"`},
		{"svcb", "example.test. 60 IN SVCB 1 svc.example.test. alpn=h2", `1 svc.example.test. alpn="h2"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dnsValueFromRR(mustWireRR(t, tt.raw)); got != tt.want {
				t.Fatalf("dnsValueFromRR(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDNSDescriptor(t *testing.T) {
	d := NewDNS("example.test").Descriptor()
	if d.Backend != "dns" || d.Target != "example.test" {
		t.Fatalf("descriptor = %+v", d)
	}
}

func wireResponse(rcode uint16, answers ...wdns.RR) *wdns.Msg {
	msg := wdns.NewMsg("example.test.", wdns.TypeA)
	msg.Rcode = rcode
	msg.Answer = answers
	return msg
}

func wireResponseFor(question string, qtype uint16, rcode uint16, answers ...wdns.RR) *wdns.Msg {
	msg := wdns.NewMsg(question, qtype)
	msg.Rcode = rcode
	msg.Answer = answers
	return msg
}

func TestDNSConditionWireRejectsQuestionMismatch(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		return wireResponseFor("other.test.", wdns.TypeA, wdns.RcodeSuccess, mustWireRR(t, "example.test. 60 IN A 192.0.2.10")), nil
	}
	result := cond.Check(t.Context())
	if result.Status != CheckUnsatisfied {
		t.Fatalf("status = %s, want unsatisfied", result.Status)
	}
}

func TestDNSValidateWireQuestionRejectsTypeClassAndCount(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	if err := cond.validateWireQuestion(&wdns.Msg{}); err == nil {
		t.Fatal("empty question succeeded")
	}
	if err := cond.validateWireQuestion(wireResponseFor("example.test.", wdns.TypeAAAA, wdns.RcodeSuccess)); err == nil {
		t.Fatal("wrong question type succeeded")
	}
	msg := wireResponseFor("example.test.", wdns.TypeA, wdns.RcodeSuccess)
	msg.Question[0].Header().Class = wdns.ClassCHAOS
	if err := cond.validateWireQuestion(msg); err == nil {
		t.Fatal("wrong question class succeeded")
	}
}

func TestDNSAdditionalValidationAndEvaluateBranches(t *testing.T) {
	bad := NewDNS("example.test")
	bad.ResolverMode = "broken"
	if err := bad.validate(); err == nil {
		t.Fatal("bad resolver mode succeeded")
	}
	bad = NewDNS("example.test")
	bad.AbsentMode = "gone"
	if err := bad.validate(); err == nil {
		t.Fatal("bad absent mode succeeded")
	}
	bad = NewDNS("example.test")
	bad.Transport = "quic"
	if err := bad.validate(); err == nil {
		t.Fatal("bad transport succeeded")
	}
	bad = NewDNS("example.test")
	bad.UDPSize = 511
	if err := bad.validate(); err == nil {
		t.Fatal("small UDP size succeeded")
	}
	bad = NewDNS("example.test")
	bad.RecordType = DNSRecordMX
	if _, err := bad.lookupSystemValues(t.Context()); err == nil {
		t.Fatal("system MX lookup succeeded")
	}
	cond := NewDNS("example.test")
	cond.Absent = true
	cond.AbsentMode = DNSAbsentNXDomain
	if result := cond.evaluate(dnsLookupResponse{NODATA: true}); result.Status != CheckUnsatisfied {
		t.Fatalf("NXDOMAIN absent with NODATA status = %s, want unsatisfied", result.Status)
	}
	cond.AbsentMode = DNSAbsentNODATA
	if result := cond.evaluate(dnsLookupResponse{NODATA: true}); result.Status != CheckSatisfied {
		t.Fatalf("NODATA absent status = %s, want satisfied", result.Status)
	}
	cond = NewDNS("example.test")
	cond.RCode = "NOERROR"
	if result := cond.evaluate(dnsLookupResponse{RCode: "NOERROR"}); result.Status != CheckSatisfied {
		t.Fatalf("rcode-only status = %s, want satisfied", result.Status)
	}
	cond = NewDNS("example.test")
	cond.MinCount = 2
	if result := cond.evaluate(dnsLookupResponse{Values: []string{"192.0.2.1"}, RCode: "NOERROR"}); result.Status != CheckUnsatisfied {
		t.Fatalf("min-count status = %s, want unsatisfied", result.Status)
	}
	cond = NewDNS("example.test")
	cond.Contains = "ready"
	if result := cond.evaluate(dnsLookupResponse{Values: []string{"warming"}, RCode: "NOERROR"}); result.Status != CheckUnsatisfied {
		t.Fatalf("contains status = %s, want unsatisfied", result.Status)
	}
}

func TestDNSAdditionalWireHelpers(t *testing.T) {
	cond := NewDNS("example.test")
	cond.ResolverMode = DNSResolverWire
	cond.Server = "127.0.0.1:53"
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		return nil, nil
	}
	if _, err := cond.lookupWire(t.Context()); err == nil {
		t.Fatal("nil wire response succeeded")
	}
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		msg := wdns.NewMsg("example.test.", wdns.TypeA)
		msg.Question = nil
		return msg, nil
	}
	if _, err := cond.lookupWire(t.Context()); err == nil {
		t.Fatal("question mismatch response succeeded")
	}
	records := []wdns.RR{
		testDNSCNAME("example.test.", "alias.example.test."),
		testDNSA("alias.example.test.", "192.0.2.10"),
		testDNSA("other.example.test.", "192.0.2.11"),
	}
	values := dnsValuesFromRRs(records, DNSRecordA, "example.test")
	if len(values) != 1 || values[0] != "192.0.2.10" {
		t.Fatalf("dnsValuesFromRRs = %v", values)
	}
	if got := dnsValueFromRR(testDNSPTR("ptr.example.test.", "target.example.test.")); got == "" {
		t.Fatal("PTR fallback value empty")
	}
	if !dnsValuesEqual("Ns.Example.", "ns.example", DNSRecordNS) {
		t.Fatal("NS names were not normalized")
	}
	if dnsValuesEqual("192.0.2.1", "192.0.2.2", DNSRecordA) {
		t.Fatal("A values unexpectedly matched")
	}
	if got := dnsRCodeString(65000); got != "RCODE65000" {
		t.Fatalf("unknown rcode = %q", got)
	}
	if _, ok := dnsRCodeValue("NO-SUCH-RCODE"); ok {
		t.Fatal("unknown rcode was valid")
	}
}

func testDNSCNAME(name, target string) wdns.RR {
	rr := &wdns.CNAME{Hdr: wdns.Header{Name: name, Class: wdns.ClassINET}}
	rr.Target = target
	return rr
}

func testDNSA(name, addr string) wdns.RR {
	rr := &wdns.A{Hdr: wdns.Header{Name: name, Class: wdns.ClassINET}}
	rr.Addr = netip.MustParseAddr(addr)
	return rr
}

func testDNSPTR(name, ptr string) wdns.RR {
	rr := &wdns.PTR{Hdr: wdns.Header{Name: name, Class: wdns.ClassINET}}
	rr.Ptr = ptr
	return rr
}

func TestDNSAdditionalLookupBranches(t *testing.T) {
	cond := NewDNS("example.test")
	cond.LookupIP = func(context.Context, string, string) ([]net.IP, error) {
		return nil, fmt.Errorf("lookup failed")
	}
	if _, err := cond.lookupIP(t.Context(), "ip4"); err == nil {
		t.Fatal("lookup IP error succeeded")
	}
	cond.LookupCNAME = func(context.Context, string) (string, error) {
		return "", fmt.Errorf("lookup failed")
	}
	if _, err := cond.lookupCNAME(t.Context()); err == nil {
		t.Fatal("lookup CNAME error succeeded")
	}
	cond.LookupTXT = func(context.Context, string) ([]string, error) {
		return nil, fmt.Errorf("lookup failed")
	}
	if _, err := cond.lookupTXT(t.Context()); err == nil {
		t.Fatal("lookup TXT error succeeded")
	}
	cond.Server = "127.0.0.1:53"
	if cond.resolver() == net.DefaultResolver {
		t.Fatal("custom DNS server used default resolver")
	}
}

func TestDNSAdditionalDefaultHelpers(t *testing.T) {
	cond := &DNSCondition{}
	if cond.recordType() != DNSRecordA {
		t.Fatalf("default record type = %s", cond.recordType())
	}
	if cond.resolverMode() != DNSResolverSystem {
		t.Fatalf("default resolver mode = %s", cond.resolverMode())
	}
	if cond.absentMode() != DNSAbsentAny {
		t.Fatalf("default absent mode = %s", cond.absentMode())
	}
	if cond.transport() != DNSTransportUDP {
		t.Fatalf("default transport = %s", cond.transport())
	}
	cond = NewDNS("example.test")
	cond.RecordType = DNSRecordTXT
	cond.ResolverMode = DNSResolverWire
	cond.AbsentMode = DNSAbsentNODATA
	cond.Transport = DNSTransportTCP
	if cond.recordType() != DNSRecordTXT || cond.resolverMode() != DNSResolverWire || cond.absentMode() != DNSAbsentNODATA || cond.transport() != DNSTransportTCP {
		t.Fatalf("explicit DNS helpers = %s/%s/%s/%s", cond.recordType(), cond.resolverMode(), cond.absentMode(), cond.transport())
	}
	if !ValidDNSRCode(" noerror ") {
		t.Fatal("trimmed/lowercase rcode was invalid")
	}
	if systemSupportsRecordType(DNSRecordMX) {
		t.Fatal("system resolver unexpectedly supports MX")
	}
	if validDNSRecordType("NOPE") || validDNSAbsentMode("gone") || validDNSTransport("tls") {
		t.Fatal("invalid DNS enum helper accepted value")
	}
}

func mustWireRR(t *testing.T, raw string) wdns.RR {
	t.Helper()
	rr, err := wdns.New(raw)
	if err != nil {
		t.Fatalf("dns.New(%q): %v", raw, err)
	}
	return rr
}
