package common

import (
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
)

// IsNotFound checks if err is or wraps ErrNotFound
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsInvalidInput checks if err is or wraps ErrInvalidInput
func IsInvalidInput(err error) bool {
	return errors.Is(err, ErrInvalidInput)
}

// IsTimeout checks if err is or wraps ErrTimeout
func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout)
}

// IsNotInitialized checks if err is or wraps ErrNotInitialized
func IsNotInitialized(err error) bool {
	return errors.Is(err, ErrNotInitialized)
}

// IsUnavailable checks if err is or wraps ErrUnavailable
func IsUnavailable(err error) bool {
	return errors.Is(err, ErrUnavailable)
}

// NotFoundError returns a wrapped not found error with context
func NotFoundError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrNotFound)
}

// InvalidInputError returns a wrapped invalid input error with context
func InvalidInputError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrInvalidInput)
}

// TimeoutError returns a wrapped timeout error with context
func TimeoutError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrTimeout)
}

// NotInitializedError returns a wrapped not initialized error with context
func NotInitializedError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrNotInitialized)
}

// UnavailableError returns a wrapped unavailable error with context
func UnavailableError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrUnavailable)
}

// ErrNodeNotFound represents a missing node error
type ErrNodeNotFound struct {
	NodeName string
}

func (e ErrNodeNotFound) Error() string {
	return fmt.Sprintf("node not found: %s", e.NodeName)
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

// NewMetricsUnavailableError creates a new metrics unavailable error
func NewMetricsUnavailableError(nodeName, reason string) error {
	return ErrMetricsUnavailable{
		NodeName: nodeName,
		Reason:   reason,
	}
}

// ErrInsufficientData represents insufficient data for analysis
type ErrInsufficientData struct {
	NodeName        string
	CurrentSamples  int
	RequiredSamples int
}

func (e ErrInsufficientData) Error() string {
	return fmt.Sprintf("insufficient data for node %s: %d/%d samples",
		e.NodeName, e.CurrentSamples, e.RequiredSamples)
}

// NewInsufficientDataError creates a new insufficient data error
func NewInsufficientDataError(nodeName string, current, required int) error {
	return ErrInsufficientData{
		NodeName:        nodeName,
		CurrentSamples:  current,
		RequiredSamples: required,
	}
}

// IsNodeNotFoundError Error type checking helpers
func IsNodeNotFoundError(err error) bool {
	var errNodeNotFound ErrNodeNotFound
	ok := errors.As(err, &errNodeNotFound)
	return ok
}

func IsMetricsUnavailableError(err error) bool {
	var errMetricsUnavailable ErrMetricsUnavailable
	ok := errors.As(err, &errMetricsUnavailable)
	return ok
}

func IsInsufficientDataError(err error) bool {
	var errInsufficientData ErrInsufficientData
	ok := errors.As(err, &errInsufficientData)
	return ok
}
