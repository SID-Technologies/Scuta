package github

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sid-technologies/scuta/lib/output"
	workerqueue "github.com/sid-technologies/scuta/lib/worker_queue"
)

const defaultMaxAttempts = 3

// doWithRetry executes an HTTP request with retry on transient errors.
// It clones the request for each attempt since the body may have been consumed.
// For GET requests (no body), it reuses the same request object.
func doWithRetry(client *http.Client, req *http.Request, maxAttempts int) (*http.Response, error) {
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	var lastErr error
	var lastResp *http.Response

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Bail immediately if context is canceled
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}

		if attempt > 0 {
			delay := workerqueue.ExponentialBackoffWithJitter(attempt-1, 1.0, 30.0)
			output.Debugf("Retry %d/%d after %.1fs", attempt+1, maxAttempts, delay)
			time.Sleep(time.Duration(delay * float64(time.Second)))
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if !isRetryableError(err) {
				return nil, err
			}
			output.Debugf("Request failed (attempt %d/%d): %v", attempt+1, maxAttempts, err)
			continue
		}

		if !isRetryableStatus(resp.StatusCode) {
			return resp, nil
		}

		// Retryable status — close body before retry
		output.Debugf("Received %d (attempt %d/%d)", resp.StatusCode, attempt+1, maxAttempts)
		resp.Body.Close()
		lastResp = resp
		lastErr = nil
	}

	// All attempts failed
	if lastResp != nil {
		return lastResp, lastErr
	}
	return nil, lastErr
}

// isRetryableError returns true for transient network errors.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Connection reset, timeout, temporary network errors
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}

	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "broken pipe") ||
		strings.Contains(errMsg, "eof")
}

// isRetryableStatus returns true for HTTP status codes that indicate transient errors.
func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
