package common

import (
	"context"
	"errors"
	"fmt"
)

// HandleContextError checks for context cancellation and wraps the error with a message
func HandleContextError(ctx context.Context, operation string) error {
	if ctx.Err() != nil {
		switch ctx.Err() {
		case context.Canceled:
			return CanceledError("%s canceled by context", operation)
		case context.DeadlineExceeded:
			return TimeoutError("%s timed out", operation)
		default:
			return fmt.Errorf("%s canceled: %w", operation, ctx.Err())
		}
	}
	return nil
}

// IsContextCanceled checks if an error is due to context cancellation
func IsContextCanceled(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, ErrCanceled) ||
		errors.Is(err, ErrTimeout)
}

// CheckContext checks if a context is done and returns an appropriate error
func CheckContext(ctx context.Context) error {
	if ctx.Err() != nil {
		return &ContextError{
			Op:  "unknown operation",
			Err: ctx.Err(),
		}
	}
	return nil
}

// WrapError wraps an error with a message and preserves context cancellation
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}
	return HandleError(err, message)
}

// ContextError wraps context cancellation errors
type ContextError struct {
	Op  string
	Err error
}

func (e *ContextError) Error() string {
	return fmt.Sprintf("context canceled during %s: %v", e.Op, e.Err)
}

func (e *ContextError) Unwrap() error {
	return e.Err
}

// Is implements errors.Is interface
func (e *ContextError) Is(target error) bool {
	_, ok := target.(*ContextError)
	if ok {
		return true
	}
	return errors.Is(e.Err, target)
}

// CheckContextWithOp checks context with operation name for better error messages
func CheckContextWithOp(ctx context.Context, op string) error {
	if ctx.Err() != nil {
		return &ContextError{
			Op:  op,
			Err: ctx.Err(),
		}
	}
	return nil
}
