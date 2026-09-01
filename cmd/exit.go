package cmd

import (
	"errors"

	"github.com/BobMali/ldsum/internal/run"
)

// exitCode maps an error to a process status: 1 for a verification the user
// must act on, 2 for a command that was wrong to run.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	// Tested before the single-error arms, not after: errors.As walks
	// Unwrap() []error, so an aggregate holding a mismatch matches those arms
	// too and would report 1 however bad the rest of the run went.
	var multi *run.VerifyErrors
	if errors.As(err, &multi) {
		worst := 0
		for _, e := range multi.Errs {
			worst = max(worst, exitCode(e))
		}
		if worst == 0 {
			// A non-nil error must never report success.
			return 2
		}
		return worst
	}
	var (
		mismatch *run.MismatchError
		missing  *run.MissingTargetError
	)
	switch {
	case errors.As(err, &mismatch):
		return 1
	case errors.As(err, &missing):
		return 1
	default:
		return 2
	}
}
