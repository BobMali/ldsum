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
	var (
		mismatch *run.MismatchError
		missing  *run.MissingTargetError
	)
	// Order is unobservable while both arms return 1. It stops being so once a
	// run can report several files at once: the worse code should win then.
	switch {
	case errors.As(err, &mismatch):
		return 1
	case errors.As(err, &missing):
		return 1
	default:
		return 2
	}
}
