package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// --remote-targets without --sums-file names no listing, so there is nothing
// for it to resolve. The guard is in Args, which Cobra runs before RunE sets
// SilenceUsage, so this is one of the few errors that still prints usage.
func TestVerifyRemoteTargetsNeedsSumsFile(t *testing.T) {
	root := newRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"verify", "--remote-targets", "f", "abc"})

	if code := execute(root); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "--remote-targets needs --sums-file") {
		t.Errorf("stderr = %q, want it to name both flags", errOut.String())
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("stdout = %q, want usage text on an argument-parsing error",
			out.String())
	}
}
