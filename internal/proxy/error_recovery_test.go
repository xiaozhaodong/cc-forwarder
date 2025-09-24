package proxy

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestErrorRecoveryManager_NewErrorRecoveryManager(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)

	if erm.maxRetries != 3 {
		t.Error("Expected maxRetries to be 3")
	}
	if erm.baseDelay != time.Second {
		t.Error("Expected baseDelay to be 1 second")
	}
	if erm.maxDelay != 30*time.Second {
		t.Error("Expected maxDelay to be 30 seconds")
	}
	if erm.backoffFactor != 2.0 {
		t.Error("Expected backoffFactor to be 2.0")
	}
}

func TestErrorRecoveryManager_ClassifyError(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)
	requestID := "test-classify-123"
	endpoint := "test-endpoint"
	group := "test-group"
	attempt := 1

	testCases := []struct {
		err           error
		expectedType  ErrorType
		description   string
		shouldRetry   bool
	}{
		{nil, ErrorTypeUnknown, "nil error", false},
		{errors.New("connection reset"), ErrorTypeNetwork, "connection reset", true},
		{errors.New("connection refused"), ErrorTypeNetwork, "connection refused", true},
		{errors.New("i/o timeout"), ErrorTypeTimeout, "i/o timeout", true},
		{errors.New("context deadline exceeded"), ErrorTypeTimeout, "deadline exceeded", true},
		{errors.New("HTTP 500 Internal Server Error"), ErrorTypeServerError, "HTTP 5xx error", true},
		{errors.New("HTTP 400 Bad Request"), ErrorTypeRateLimit, "HTTP 400 error (now rate limit)", true},
		{errors.New("HTTP 404 Not Found"), ErrorTypeHTTP, "HTTP 4xx error (non-400)", false},
		{errors.New("unauthorized access"), ErrorTypeAuth, "auth error", false},
		{errors.New("rate limit exceeded"), ErrorTypeRateLimit, "rate limit", true},
		{errors.New("stream parsing error"), ErrorTypeStream, "stream error", true},
		{errors.New("unknown issue"), ErrorTypeUnknown, "unknown error", true},
	}

	for _, tc := range testCases {
		errorCtx := erm.ClassifyError(tc.err, requestID, endpoint, group, attempt)

		if errorCtx.ErrorType != tc.expectedType {
			t.Errorf("For error '%v', expected type %v, got %v", tc.err, tc.expectedType, errorCtx.ErrorType)
		}

		if errorCtx.RequestID != requestID {
			t.Errorf("RequestID not set correctly")
		}

		if errorCtx.EndpointName != endpoint {
			t.Errorf("EndpointName not set correctly")
		}

		if errorCtx.AttemptCount != attempt {
			t.Errorf("AttemptCount not set correctly")
		}

		shouldRetry := erm.ShouldRetry(errorCtx)
		if tc.description == "HTTP 4xx error (non-400)" && shouldRetry {
			t.Errorf("HTTP 4xx errors (non-400) should not be retryable")
		}
		if tc.description == "HTTP 400 error (now rate limit)" && !shouldRetry {
			t.Errorf("HTTP 400 errors should now be retryable as rate limit")
		}
		if tc.description == "auth error" && shouldRetry {
			t.Errorf("Auth errors should not be retryable")
		}
	}
}

func TestErrorRecoveryManager_ShouldRetry(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)

	testCases := []struct {
		errorType     ErrorType
		attemptCount  int
		maxRetries    int
		expected      bool
		description   string
	}{
		{ErrorTypeNetwork, 1, 3, true, "network error within retry limit"},
		{ErrorTypeNetwork, 5, 3, false, "network error exceeds retry limit"},
		{ErrorTypeTimeout, 2, 3, true, "timeout error within retry limit"},
		{ErrorTypeAuth, 1, 3, false, "auth error not retryable"},
		{ErrorTypeHTTP, 1, 3, false, "general HTTP error not retryable"},
		{ErrorTypeRateLimit, 2, 3, true, "rate limit retryable"},
		{ErrorTypeStream, 1, 3, true, "stream error retryable"},
		{ErrorTypeParsing, 1, 3, true, "parsing error retryable"},
		{ErrorTypeUnknown, 1, 3, true, "unknown error retryable once"},
		{ErrorTypeUnknown, 3, 3, false, "unknown error exceeds safe retry limit"},
	}

	for _, tc := range testCases {
		errorCtx := &ErrorContext{
			RequestID:     "test-retry-456",
			ErrorType:     tc.errorType,
			AttemptCount:  tc.attemptCount,
			MaxRetries:    tc.maxRetries,
			OriginalError: errors.New("test error"),
		}

		result := erm.ShouldRetry(errorCtx)
		if result != tc.expected {
			t.Errorf("%s: expected %v, got %v", tc.description, tc.expected, result)
		}
	}
}

func TestErrorRecoveryManager_CalculateBackoffDelay(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)

	testCases := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
	}

	for _, tc := range testCases {
		delay := erm.calculateBackoffDelay(tc.attempt)
		if delay != tc.expected {
			t.Errorf("For attempt %d, expected delay %v, got %v", tc.attempt, tc.expected, delay)
		}
	}

	// Test max delay limit
	largeAttempt := 10
	delay := erm.calculateBackoffDelay(largeAttempt)
	if delay > erm.maxDelay {
		t.Errorf("Delay should not exceed maxDelay, got %v", delay)
	}
}

func TestErrorRecoveryManager_IsNetworkError(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)

	testCases := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{errors.New("connection reset"), true},
		{errors.New("connection refused"), true},
		{errors.New("Connection REFUSED"), true}, // Case insensitive
		{errors.New("i/o timeout"), false}, // This is a timeout error, not network
		{errors.New("network is unreachable"), true},
		{errors.New("no route to host"), true},
		{errors.New("broken pipe"), true},
		{errors.New("eof"), true},
		{errors.New("unexpected eof"), true},
		{errors.New("random error"), false},
		{&net.OpError{Op: "dial"}, true},
		{&net.DNSError{}, true},
	}

	for _, tc := range testCases {
		result := erm.isNetworkError(tc.err)
		if result != tc.expected {
			t.Errorf("isNetworkError(%v) = %v, expected %v", tc.err, result, tc.expected)
		}
	}
}

func TestErrorRecoveryManager_IsTimeoutError(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)

	testCases := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{context.DeadlineExceeded, true},
		{errors.New("timeout occurred"), true},
		{errors.New("i/o timeout"), true},
		{errors.New("context deadline exceeded"), true},
		{errors.New("read timeout"), true},
		{errors.New("write timeout"), true},
		{errors.New("random error"), false},
	}

	for _, tc := range testCases {
		result := erm.isTimeoutError(tc.err)
		if result != tc.expected {
			t.Errorf("isTimeoutError(%v) = %v, expected %v", tc.err, result, tc.expected)
		}
	}
}

func TestErrorRecoveryManager_ExecuteRetry(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)
	
	// Test with no delay
	errorCtx := &ErrorContext{
		RequestID:      "test-execute-789",
		RetryableAfter: 0,
		AttemptCount:   1,
	}

	ctx := context.Background()
	err := erm.ExecuteRetry(ctx, errorCtx)
	if err != nil {
		t.Errorf("ExecuteRetry should not return error for no delay, got: %v", err)
	}

	// Test with short delay
	errorCtx.RetryableAfter = 10 * time.Millisecond
	start := time.Now()
	err = erm.ExecuteRetry(ctx, errorCtx)
	elapsed := time.Since(start)
	
	if err != nil {
		t.Errorf("ExecuteRetry should not return error for short delay, got: %v", err)
	}
	if elapsed < errorCtx.RetryableAfter {
		t.Errorf("ExecuteRetry should wait for the specified delay")
	}

	// Test with context cancellation
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	
	errorCtx.RetryableAfter = time.Second
	err = erm.ExecuteRetry(cancelCtx, errorCtx)
	if err != context.Canceled {
		t.Errorf("ExecuteRetry should return context.Canceled when context is cancelled, got: %v", err)
	}
}

func TestErrorRecoveryManager_SetRetryPolicy(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)

	newMaxRetries := 5
	newBaseDelay := 2 * time.Second
	newMaxDelay := 60 * time.Second
	newBackoffFactor := 1.5

	erm.SetRetryPolicy(newMaxRetries, newBaseDelay, newMaxDelay, newBackoffFactor)

	if erm.maxRetries != newMaxRetries {
		t.Errorf("MaxRetries not updated correctly")
	}
	if erm.baseDelay != newBaseDelay {
		t.Errorf("BaseDelay not updated correctly")
	}
	if erm.maxDelay != newMaxDelay {
		t.Errorf("MaxDelay not updated correctly")
	}
	if erm.backoffFactor != newBackoffFactor {
		t.Errorf("BackoffFactor not updated correctly")
	}
}

func TestErrorRecoveryManager_GetErrorTypeName(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)

	testCases := []struct {
		errorType ErrorType
		expected  string
	}{
		{ErrorTypeNetwork, "网络"},
		{ErrorTypeTimeout, "超时"},
		{ErrorTypeHTTP, "HTTP"},
		{ErrorTypeStream, "流处理"},
		{ErrorTypeAuth, "认证"},
		{ErrorTypeRateLimit, "限流"},
		{ErrorTypeParsing, "解析"},
		{ErrorTypeUnknown, "未知"},
	}

	for _, tc := range testCases {
		result := erm.getErrorTypeName(tc.errorType)
		if result != tc.expected {
			t.Errorf("getErrorTypeName(%v) = %s, expected %s", tc.errorType, result, tc.expected)
		}
	}
}

func TestErrorRecoveryManager_RecoverFromPartialData(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)
	requestID := "test-recover-101"

	// Test with empty data
	erm.RecoverFromPartialData(requestID, nil, time.Second)

	// Test with data containing token info
	tokenData := []byte(`{"usage": {"input_tokens": 100, "output_tokens": 50}}`)
	erm.RecoverFromPartialData(requestID, tokenData, time.Second)

	// Test with regular data
	regularData := []byte("some response data without token info")
	erm.RecoverFromPartialData(requestID, regularData, time.Second)
}

// mockNetError for testing
type mockSyscallError struct {
	errno syscall.Errno
}

func (e *mockSyscallError) Error() string {
	return e.errno.Error()
}

func (e *mockSyscallError) Unwrap() error {
	return e.errno
}

func TestErrorRecoveryManager_ClassifyError_SyscallErrors(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)

	testCases := []struct {
		errno    syscall.Errno
		expected ErrorType
	}{
		{syscall.ECONNREFUSED, ErrorTypeNetwork},
		{syscall.ECONNRESET, ErrorTypeNetwork},
		{syscall.ETIMEDOUT, ErrorTypeTimeout}, // This is a timeout error
		{syscall.ENETUNREACH, ErrorTypeNetwork},
		{syscall.EHOSTUNREACH, ErrorTypeNetwork},
	}

	for _, tc := range testCases {
		mockErr := &mockSyscallError{errno: tc.errno}
		errorCtx := erm.ClassifyError(mockErr, "test", "endpoint", "group", 1)
		
		if errorCtx.ErrorType != tc.expected {
			t.Errorf("For syscall error %v, expected %v, got %v", tc.errno, tc.expected, errorCtx.ErrorType)
		}
	}
}