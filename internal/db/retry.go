package db

import (
	"strings"
	"time"
)

const (
	maxRetries = 5
	retryDelay = 100 * time.Millisecond
)

// isRetryableError checks if a MySQL error is a deadlock or lock timeout.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// MySQL 1205: Lock wait timeout exceeded
	// MySQL 1213: Deadlock found when trying to get lock
	return strings.Contains(msg, "1205") || strings.Contains(msg, "1213") ||
		strings.Contains(msg, "deadlock") || strings.Contains(msg, "lock wait timeout")
}

// WithRetry executes fn with retry logic for deadlocks and lock timeouts.
func WithRetry(fn func() error) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil || !isRetryableError(err) {
			return err
		}
		time.Sleep(retryDelay)
	}
	return err
}
