package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sha256 of "abc", from FIPS 180-4.
const abcSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

// runCLI drives the real command tree with buffers in place of the process
// streams, and undoes the global state Cobra keeps between runs.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		verifyAlgorithm = ""
	})

	err = rootCmd.Execute()
	return out.String(), errOut.String(), err
}

func fixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestVerifyCommand(t *testing.T) {
	t.Run("match", func(t *testing.T) {
		path := fixture(t, "abc")

		stdout, stderr, err := runCLI(t, "verify", path, abcSHA256)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if want := path + ": OK\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("explicit algorithm", func(t *testing.T) {
		path := fixture(t, "abc")

		stdout, _, err := runCLI(t, "verify", "--algo", "sha256", path, abcSHA256)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if want := path + ": OK\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		path := fixture(t, "abc")
		wrong := strings.Repeat("0", 64)

		stdout, stderr, err := runCLI(t, "verify", path, wrong)
		if err == nil {
			t.Fatal("Execute() = nil error, want a mismatch error")
		}
		if want := path + ": FAILED\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if !strings.Contains(stderr, "expected: "+wrong) {
			t.Errorf("stderr = %q, want the expected digest", stderr)
		}
		if strings.Contains(stderr, "Usage:") {
			t.Errorf("stderr = %q, want no usage dump on a mismatch", stderr)
		}
	})

	t.Run("wrong argument count", func(t *testing.T) {
		path := fixture(t, "abc")

		if _, _, err := runCLI(t, "verify", path); err == nil {
			t.Error("Execute() = nil error, want an argument-count error")
		}
	})
}
