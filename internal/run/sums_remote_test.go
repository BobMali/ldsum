package run

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// The guard has to run before the checksum file is opened, or a missing local
// file reports "no such file" and never mentions the flag that was wrong.
func TestVerifySumsRemoteTargetsNeedsARemoteSumsFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "SHA256SUMS")

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile:      missing,
		RemoteTargets: true,
	})
	if err == nil {
		t.Fatal("VerifySums() error = nil, want the flag to be refused")
	}
	if !strings.Contains(err.Error(), "--remote-targets") {
		t.Errorf("error = %q, want it to name the flag", err)
	}
	if strings.Contains(err.Error(), "no such file") {
		t.Errorf("error = %q, want the flag error rather than the open error", err)
	}
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q, want both empty",
			out.String(), errOut.String())
	}
}
