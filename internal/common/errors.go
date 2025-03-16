package common

import (
	"context"
	"errors"
	"fmt"
)

// Common error types
var (
	// ErrNotFound indicates a resource was not found
	ErrNotFound = errors.New("resource not found")

	// ErrInvalidInput indicates invalid input parameters
	ErrInvalidInput = errors.New("invalid input parameter")

	// ErrTimeout indicates an operation timed out
	ErrTimeout = errors.New("operation timed out")

	// ErrNotInitialized indicates a component is not initialized
	ErrNotInitialized = errors.New("component not initialized")

	// ErrUnavailable indicates a service is unavailable
	ErrUnavailable = errors.New("service unavailable")

	// ErrCanceled indicates an operation was canceled
	ErrCanceled = errors.New("operation canceled")

	// ErrFailedPrecondition indicates a condition required for the operation was not met
	ErrFailedPrecondition = errors.New("failed precondition")

	// ErrInternal indicates an internal error
	ErrInternal = errors.New("internal error")
)

// NotFoundError returns a wrapped not found error with context
func NotFoundError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrNotFound)
}

// InvalidInputError returns a wrapped invalid input error with context
func InvalidInputError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrInvalidInput)
}

// NotInitializedError returns a wrapped not initialized error with context
func NotInitializedError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrNotInitialized)
}

// UnavailableError returns a wrapped unavailable error with context
func UnavailableError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrUnavailable)
}

// TimeoutError returns a wrapped timeout error with context
func TimeoutError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrTimeout)
}

// CanceledError returns a wrapped canceled error with context
func CanceledError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrCanceled)
}

// InternalError returns a wrapped internal error with context
func InternalError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrInternal)
}

// FailedPreconditionError returns a wrapped failed precondition error with context
func FailedPreconditionError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrFailedPrecondition)
}

// ErrNodeNotFound represents a missing node error
type ErrNodeNotFound struct {
	NodeName string
}

func (e ErrNodeNotFound) Error() string {
	return fmt.Sprintf("node not found: %s", e.NodeName)
}

// Is implements the errors.Is interface for type matching
func (e ErrNodeNotFound) Is(target error) bool {
	_, ok := target.(ErrNodeNotFound)
	return ok || errors.Is(target, ErrNotFound)
}

// NewNodeNotFoundError creates a new node not found error
func NewNodeNotFoundError(nodeName string) error {
	return ErrNodeNotFound{NodeName: nodeName}
}

// ErrMetricsUnavailable represents metrics collection failures
type ErrMetricsUnavailable struct {
	NodeName string
	Reason   string
}

func (e ErrMetricsUnavailable) Error() string {
	return fmt.Sprintf("metrics unavailable for node %s: %s", e.NodeName, e.Reason)
}

// Is implements the errors.Is interface for type matching
func (e ErrMetricsUnavailable) Is(target error) bool {
	_, ok := target.(ErrMetricsUnavailable)
	return ok || errors.Is(target, ErrUnavailable)
}

// ErrInsufficientData represents cases where there's not enough data
type ErrInsufficientData struct {
	Resource string
	Reason   string
}

func (e ErrInsufficientData) Error() string {
	return fmt.Sprintf("insufficient data for %s: %s", e.Resource, e.Reason)
}

// Is implements the errors.Is interface for type matching
func (e ErrInsufficientData) Is(target error) bool {
	_, ok := target.(ErrInsufficientData)
	return ok || errors.Is(target, ErrInvalidInput)
}

// IsNodeNotFoundError checks if an error is an ErrNodeNotFound
func IsNodeNotFoundError(err error) bool {
	var nodeErr ErrNodeNotFound
	return errors.As(err, &nodeErr) || errors.Is(err, ErrNotFound)
}

// HandleError provides standardized error handling with the option to wrap the error
// If wrapf is empty, the original error will be returned unchanged
func HandleError(err error, wrapf string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	if wrapf == "" {
		return err
	}

	// Check for context cancellation errors
	if IsContextCanceled(err) {
		return fmt.Errorf("%s: %w", fmt.Sprintf(wrapf, args...), err)
	}

	// Check if error matches any of our defined error types
	switch {
	case errors.Is(err, ErrNotFound):
		return fmt.Errorf("%s: %w", fmt.Sprintf(wrapf, args...), err)
	case errors.Is(err, ErrInvalidInput):
		return fmt.Errorf("%s: %w", fmt.Sprintf(wrapf, args...), err)
	case errors.Is(err, ErrTimeout):
		return fmt.Errorf("%s: %w", fmt.Sprintf(wrapf, args...), err)
	case errors.Is(err, ErrNotInitialized):
		return fmt.Errorf("%s: %w", fmt.Sprintf(wrapf, args...), err)
	case errors.Is(err, ErrUnavailable):
		return fmt.Errorf("%s: %w", fmt.Sprintf(wrapf, args...), err)
	case errors.Is(err, ErrCanceled):
		return fmt.Errorf("%s: %w", fmt.Sprintf(wrapf, args...), err)
	case errors.Is(err, ErrFailedPrecondition):
		return fmt.Errorf("%s: %w", fmt.Sprintf(wrapf, args...), err)
	case errors.Is(err, ErrInternal):
		return fmt.Errorf("%s: %w", fmt.Sprintf(wrapf, args...), err)
	default:
		// For unknown error types, preserve error chain using %w
		return fmt.Errorf("%s: %w", fmt.Sprintf(wrapf, args...), err)
	}
}

// ProcessContextError provides standardized context error handling
func ProcessContextError(ctx context.Context, operation string) error {
	if ctx.Err() == nil {
		return nil
	}

	switch ctx.Err() {
	case context.Canceled:
		return CanceledError("%s canceled by context", operation)
	case context.DeadlineExceeded:
		return TimeoutError("%s timed out", operation)
	default:
		return fmt.Errorf("%s failed: %w", operation, ctx.Err())
	}
}

// MustCheck checks if the error is non-nil and panics if it is
// This should only be used during setup when a panic is appropriate
func MustCheck(err error) {
	if err != nil {
		panic(err)
	}
}

// InsufficientDataError Define a custom error type (if needed)
type InsufficientDataError struct {
	Message  string
	Provided int
	Required int
}

// Implement the error interface
func (e *InsufficientDataError) Error() string {
	return fmt.Sprintf("%s: provided %d, requires at least %d", e.Message, e.Provided, e.Required)
}

// NewInsufficientDataError creates a new InsufficientDataError.
func NewInsufficientDataError(message string, provided, required int) error {
	return &InsufficientDataError{
		Message:  message,
		Provided: provided,
		Required: required,
	}
}
