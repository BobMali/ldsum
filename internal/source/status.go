package source

import (
	"io/fs"
	"net/http"
)

// StatusError reports a response that was not a success.
type StatusError struct {
	// URL is the URL that produced the response — after any redirects, so it
	// names where the request ended rather than where it started.
	URL    string
	Status string // resp.Status, e.g. "404 Not Found"
	Code   int
}

func (e *StatusError) Error() string { return e.URL + ": " + e.Status }

// Is reports a not-found response as fs.ErrNotExist, so a caller that already
// tells a missing file from an unreadable one needs no second branch for URLs.
func (e *StatusError) Is(target error) bool {
	return target == fs.ErrNotExist &&
		(e.Code == http.StatusNotFound || e.Code == http.StatusGone)
}
