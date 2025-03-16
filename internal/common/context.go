package common

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// CheckContext checks if a context is done and returns an appropriate error
func CheckContext(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// HandleContextError checks for context cancellation and wraps the error with a message
func HandleContextError(ctx context.Context, operation string) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%s canceled: %w", operation, ctx.Err())
	}
	return nil
}

// IsContextCanceled checks if an error is due to context cancellation
func IsContextCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// WrapError wraps an error with a message and preserves context cancellation
func WrapError(err error, message string) error {
	if IsContextCanceled(err) {
		return fmt.Errorf("%s: %w", message, err)
	}
	return fmt.Errorf("%s: %v", message, err)
}

// WithTimeout runs a function with a timeout and returns its result or a timeout error
func WithTimeout(ctx context.Context, timeout time.Duration, operation func(context.Context) error) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- operation(timeoutCtx)
	}()

	select {
	case err := <-done:
		return err
	case <-timeoutCtx.Done():
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("operation timed out after %v", timeout)
		}
		return timeoutCtx.Err()
	}
}
