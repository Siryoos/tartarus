package charon

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	// ErrNoHealthyShores indicates all backends are unavailable
	ErrNoHealthyShores = errors.New("no healthy shores available - the river cannot be crossed")

	// ErrRateLimitExceeded indicates rate limit has been exceeded
	ErrRateLimitExceeded = errors.New("rate limit exceeded - the ferryman demands patience")

	// ErrCircuitOpen indicates circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker open - passage denied")

	// ErrShoreNotFound indicates the requested shore doesn't exist
	ErrShoreNotFound = errors.New("shore not found")

	// ErrShoreAlreadyExists indicates a shore with the same ID already exists
	ErrShoreAlreadyExists = errors.New("shore already exists")

	// ErrInvalidConfig indicates configuration is invalid
	ErrInvalidConfig = errors.New("invalid configuration")
)

// CrossingError represents an error during request crossing.
type CrossingError struct {
	Code    int    // HTTP status code
	Message string // Error message
	Err     error  // Underlying error
}

func (e *CrossingError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *CrossingError) Unwrap() error {
	return e.Err
}

// HTTPStatusCode returns the appropriate HTTP status code for the error.
func (e *CrossingError) HTTPStatusCode() int {
	return e.Code
}

// NewCrossingError creates a new crossing error.
func NewCrossingError(code int, message string, err error) *CrossingError {
	return &CrossingError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// ToHTTPError converts an error to a CrossingError with appropriate HTTP status.
func ToHTTPError(err error) *CrossingError {
	if err == nil {
		return nil
	}

	// Check if already a CrossingError
	var ce *CrossingError
	if errors.As(err, &ce) {
		return ce
	}

	// Map known errors to HTTP status codes
	switch {
	case errors.Is(err, ErrNoHealthyShores):
		return NewCrossingError(http.StatusServiceUnavailable, err.Error(), err)
	case errors.Is(err, ErrRateLimitExceeded):
		return NewCrossingError(http.StatusTooManyRequests, err.Error(), err)
	case errors.Is(err, ErrCircuitOpen):
		return NewCrossingError(http.StatusServiceUnavailable, err.Error(), err)
	case errors.Is(err, ErrShoreNotFound):
		return NewCrossingError(http.StatusNotFound, err.Error(), err)
	case errors.Is(err, ErrShoreAlreadyExists):
		return NewCrossingError(http.StatusConflict, err.Error(), err)
	case errors.Is(err, ErrInvalidConfig):
		return NewCrossingError(http.StatusInternalServerError, err.Error(), err)
	default:
		return NewCrossingError(http.StatusInternalServerError, "internal ferry error", err)
	}
}
