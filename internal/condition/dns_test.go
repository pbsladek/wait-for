package condition

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	wdns "codeberg.org/miekg/dns"
)

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

func TestDNSConditionWireNXDomainAbsentMode(t *testing.T) {
	cond := NewDNS("missing.example.test")
	cond.ResolverMode = DNSResolverWire
	cond.Absent = true
	cond.AbsentMode = DNSAbsentNXDomain
	cond.WireExchange = func(context.Context, *wdns.Msg, string, string) (*wdns.Msg, error) {
		return wireResponse(wdns.RcodeNameError), nil
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

func TestDNSDescriptor(t *testing.T) {
	d := NewDNS("example.test").Descriptor()
	if d.Backend != "dns" || d.Target != "example.test" {
		t.Fatalf("descriptor = %+v", d)
	}
}

func wireResponse(rcode uint16, answers ...wdns.RR) *wdns.Msg {
	return &wdns.Msg{
		MsgHeader: wdns.MsgHeader{Rcode: rcode},
		Answer:    answers,
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
