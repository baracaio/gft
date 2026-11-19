package domain

import "fmt"

// PermanentError wraps validation failures that are deterministic and must not
// be retried. After maxReceiveCount exhaustion, SQS routes the message to DLQ.
type PermanentError struct {
	Field   string
	Message string
}

func (e *PermanentError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("permanent error (field=%q): %s", e.Field, e.Message)
	}
	return "permanent error: " + e.Message
}

// TransientError wraps retriable failures: version conflict after all internal
// retries, DynamoDB throttle, or network errors. The SQS message becomes
// visible again for redelivery.
type TransientError struct {
	Cause error
}

func (e *TransientError) Error() string { return fmt.Sprintf("transient error: %v", e.Cause) }
func (e *TransientError) Unwrap() error { return e.Cause }
