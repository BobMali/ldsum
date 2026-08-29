package cmd

import (
	"errors"
	"io/fs"

	"github.com/BobMali/ldsum/internal/run"
)

// exitCode maps an error to a process status: 1 for a verification the user
// must act on, 2 for a command that was wrong to run.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var mismatch *run.MismatchError
	switch {
	case errors.As(err, &mismatch):
		return 1
	case errors.Is(err, fs.ErrNotExist):
		return 1
	default:
		return 2
	}
}
