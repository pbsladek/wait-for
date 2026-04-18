package condition

import "testing"

func TestExecConditionSatisfied(t *testing.T) {
	cond := NewExec([]string{"/bin/sh", "-c", "printf '{\"ready\":true}\\n'"})
	cond.OutputJSONPath = ".ready == true"

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("Satisfied = false, err = %v, detail = %q", result.Err, result.Detail)
	}
}

func TestExecConditionExitCodeMismatch(t *testing.T) {
	cond := NewExec([]string{"/bin/sh", "-c", "exit 7"})

	result := cond.Check(t.Context())
	if result.Status == CheckSatisfied {
		t.Fatal("Satisfied = true, want false")
	}
	if result.Err == nil {
		t.Fatal("Err = nil, want exit code error")
	}
}

func TestExecConditionExpectedExitCode(t *testing.T) {
	cond := NewExec([]string{"/bin/sh", "-c", "exit 7"})
	cond.ExpectedExitCode = 7

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("Satisfied = false, err = %v", result.Err)
	}
}

func TestExecConditionCwdEnvAndOutputLimit(t *testing.T) {
	dir := t.TempDir()
	cond := NewExec([]string{"/bin/sh", "-c", "printf '%s:%s:abcdef' \"$PWD\" \"$WAITFOR_TEST\""})
	cond.Cwd = dir
	cond.Env = []string{"WAITFOR_TEST=yes"}
	cond.OutputContains = ":yes:abc"
	cond.MaxOutputBytes = int64(len(dir) + len(":yes:abc"))

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("Satisfied = false, err = %v, detail = %q", result.Err, result.Detail)
	}
}

func TestExecConditionCommandNotFoundIsFatal(t *testing.T) {
	cond := NewExec([]string{"/definitely/missing/waitfor-command"})

	result := cond.Check(t.Context())
	if result.Err == nil {
		t.Fatal("Err = nil, want command error")
	}
	if result.Status != CheckFatal {
		t.Fatalf("Status = %s, want %s", result.Status, CheckFatal)
	}
}
