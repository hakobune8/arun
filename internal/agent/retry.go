// Package agent provides core agent interfaces and base implementations for coding agents.
package agent

// RetryConfig holds configuration for the retry behavior on failed tasks.
type RetryConfig struct {
	MaxRetries int
}

// RetryHandler determines whether to retry a task based on its configuration and attempt status.
type RetryHandler struct {
	config RetryConfig
}

// NewRetryHandler creates a new RetryHandler with the given configuration.
func NewRetryHandler(config RetryConfig) *RetryHandler {
	return &RetryHandler{config: config}
}

// ShouldRetry returns true if the task should be retried based on the current attempt count and failure status.
func (h *RetryHandler) ShouldRetry(attempt int, testFailed bool, lintFailed bool) bool {
	if attempt >= h.config.MaxRetries {
		return false
	}
	return testFailed || lintFailed
}
