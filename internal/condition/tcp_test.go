package condition

import (
	"net"
	"testing"
)

func TestTCPConditionSatisfied(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	result := NewTCP(listener.Addr().String()).Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("Satisfied = false, err = %v", result.Err)
	}
	<-done
}
