package backend

import "errors"

// ErrNotImplemented is returned when a BucketClass.backend is known but has no provider yet (e.g. AWS).
var ErrNotImplemented = errors.New("storage backend not implemented")
