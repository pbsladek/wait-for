package condition

import (
	"context"
	"fmt"
	"testing"
)

func TestDockerConditionRunningSatisfied(t *testing.T) {
	cond := NewDocker("api")
	cond.Inspect = func(_ context.Context, container string) (DockerState, error) {
		if container != "api" {
			t.Fatalf("container = %q, want api", container)
		}
		return DockerState{Status: "running", Running: true}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDockerConditionStatusUnsatisfied(t *testing.T) {
	cond := NewDocker("api")
	cond.Inspect = func(context.Context, string) (DockerState, error) {
		return DockerState{Status: "created"}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckUnsatisfied {
		t.Fatalf("status = %s, want unsatisfied", result.Status)
	}
}

func TestDockerConditionAnyStatus(t *testing.T) {
	cond := NewDocker("api")
	cond.Status = "any"
	cond.Inspect = func(context.Context, string) (DockerState, error) {
		return DockerState{Status: "exited"}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDockerConditionHealthy(t *testing.T) {
	cond := NewDocker("api")
	cond.Health = "healthy"
	cond.Inspect = func(context.Context, string) (DockerState, error) {
		return DockerState{
			Status: "running",
			Health: &DockerHealth{Status: "healthy"},
		}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDockerConditionHealthMissing(t *testing.T) {
	cond := NewDocker("api")
	cond.Health = "healthy"
	cond.Inspect = func(context.Context, string) (DockerState, error) {
		return DockerState{Status: "running"}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckUnsatisfied {
		t.Fatalf("status = %s, want unsatisfied", result.Status)
	}
}

func TestDockerConditionNoHealthSatisfied(t *testing.T) {
	cond := NewDocker("api")
	cond.Health = "none"
	cond.Inspect = func(context.Context, string) (DockerState, error) {
		return DockerState{Status: "running"}, nil
	}

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("status = %s, err = %v", result.Status, result.Err)
	}
}

func TestDockerConditionInspectErrorUnsatisfied(t *testing.T) {
	cond := NewDocker("missing")
	cond.Inspect = func(context.Context, string) (DockerState, error) {
		return DockerState{}, fmt.Errorf("no such container")
	}

	result := cond.Check(t.Context())
	if result.Status != CheckUnsatisfied {
		t.Fatalf("status = %s, want unsatisfied", result.Status)
	}
}

func TestDockerConditionEmptyContainerFatal(t *testing.T) {
	result := NewDocker(" ").Check(t.Context())
	if result.Status != CheckFatal {
		t.Fatalf("status = %s, want fatal", result.Status)
	}
}

func TestDockerDescriptor(t *testing.T) {
	d := NewDocker("api").Descriptor()
	if d.Backend != "docker" || d.Target != "api" {
		t.Fatalf("descriptor = %+v", d)
	}
}
