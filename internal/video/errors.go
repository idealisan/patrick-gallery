package video

import "errors"

// errBackendUnavailable is returned by a backend constructor when its native
// library cannot be loaded, signalling New() to try the next backend.
var errBackendUnavailable = errors.New("video backend not available")
