package storage

import "errors"

var (
	// ErrInvalidBackend is returned when an invalid backend type is specified
	ErrInvalidBackend = errors.New("invalid storage backend")

	// ErrNotEnabled is returned when storage is not enabled
	ErrNotEnabled = errors.New("storage is not enabled")

	// ErrNotFound is returned when a query or entity is not found
	ErrNotFound = errors.New("not found")

	// ErrInvalidConfig is returned when configuration is invalid
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrConnectionFailed is returned when connection to storage fails
	ErrConnectionFailed = errors.New("connection failed")

	// ErrQueryFailed is returned when a query fails
	ErrQueryFailed = errors.New("query failed")

	// ErrBufferFull is returned when the write buffer is full
	ErrBufferFull = errors.New("buffer full")

	// ErrClosed is returned when attempting to use a closed storage
	ErrClosed = errors.New("storage is closed")
)
