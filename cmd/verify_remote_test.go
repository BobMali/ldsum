package cmd

import (
	"bytes"
	"os"
	"path/filepath"
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

// A local --sums-file trips the run guard, not the Args one, so this proves
// the flag actually reached SumsOptions rather than just parsing.
func TestVerifyRemoteTargetsNeedsARemoteSumsFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "SHA256SUMS")

	root := newRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"verify", "--remote-targets", "--sums-file", missing})

	if code := execute(root); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "needs a URL for --sums-file") {
		t.Errorf("stderr = %q, want the run guard's message", errOut.String())
	}
	if strings.Contains(out.String(), "Usage:") {
		t.Errorf("stdout = %q, want no usage text: this error arises after RunE silences it",
			out.String())
	}
}

// An explicit --remote-targets=false asks for the behaviour a run without a
// checksum file already has, so refusing it says no to a request that was
// never in conflict — and breaks a script passing the flag from a variable.
func TestVerifyRemoteTargetsFalseNeedsNoSumsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	root := newRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"verify", "--remote-targets=false", path, abcSHA256})

	if code := execute(root); code != 0 {
		t.Errorf("exit = %d, want 0\nstderr: %s", code, errOut.String())
	}
	if want := path + ": OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}
