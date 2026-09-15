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

// An unfetchable scheme is IsRemote's own error, not "needs a URL": the user
// did give a URL, just one this tool cannot fetch, and conflating the two
// would blame the wrong thing.
func TestVerifySumsRemoteTargetsUnsupportedScheme(t *testing.T) {
	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile:      "ftp://host/SHA256SUMS",
		RemoteTargets: true,
	})
	if err == nil {
		t.Fatal("VerifySums() error = nil, want the unsupported scheme to be refused")
	}
	if !strings.Contains(err.Error(), "unsupported scheme") {
		t.Errorf("error = %q, want it to name the unsupported scheme", err)
	}
	if strings.Contains(err.Error(), "needs a URL") {
		t.Errorf("error = %q, want the scheme error, not the flag error", err)
	}
}
