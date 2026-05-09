package condition

import (
	"context"
	"fmt"
	"net"
	"time"
)

type TCPCondition struct {
	Address        string
	AttemptTimeout time.Duration
}

func NewTCP(address string) *TCPCondition {
	return &TCPCondition{Address: address, AttemptTimeout: 2 * time.Second}
}

func (c *TCPCondition) Descriptor() Descriptor {
	return Descriptor{Backend: "tcp", Target: c.Address}
}

func (c *TCPCondition) Check(ctx context.Context) Result {
	if err := validateTCPConfig(c); err != nil {
		return Fatal(err)
	}
	timeout := c.AttemptTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", c.Address)
	if err != nil {
		return Unsatisfied("", err)
	}
	_ = conn.Close()
	return Satisfied("connection established")
}

func validateTCPConfig(c *TCPCondition) error {
	if c.Address == "" {
		return fmt.Errorf("tcp address is required")
	}
	_, port, err := net.SplitHostPort(c.Address)
	if err != nil {
		return fmt.Errorf("invalid tcp address %q: %w", c.Address, err)
	}
	if port == "" {
		return fmt.Errorf("invalid tcp address %q: missing port", c.Address)
	}
	return nil
}
