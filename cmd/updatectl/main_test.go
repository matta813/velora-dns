package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdatectlCommandsPersistAndReportState(t *testing.T) {
	if os.Getenv("GO_WANT_UPDATECTL_HELPER") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"updatectl"}, os.Args[i+1:]...)
				main()
				return
			}
		}
		os.Exit(2)
	}

	stateFile := filepath.Join(t.TempDir(), "state.json")
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=TestUpdatectlCommandsPersistAndReportState", "--")
		cmd.Args = append(cmd.Args, args...)
		cmd.Env = append(os.Environ(), "GO_WANT_UPDATECTL_HELPER=1", "VELORA_UPDATE_STATE_FILE="+stateFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("updatectl %v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	if got := run("status"); !strings.Contains(got, `"state":"idle"`) {
		t.Fatalf("initial status = %s", got)
	}
	begin := run("begin", "v1.0.0", "v1.1.0", "systemd")
	if !strings.Contains(begin, `"from_version":"v1.0.0"`) {
		t.Fatalf("begin output = %s", begin)
	}
	if got := run("status"); !strings.Contains(got, `"state":"downloading"`) {
		t.Fatalf("active status = %s", got)
	}
}
