package middleware

import (
	"net/http"
	"strconv"
	"time"
)

// MetricsCollector records HTTP request metrics.
type MetricsCollector interface {
	RecordRequest(method, path string, statusCode int, duration time.Duration)
}

// Metrics returns middleware that records request metrics.
func Metrics(collector MetricsCollector) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			if collector != nil {
				collector.RecordRequest(r.Method, r.URL.Path, sw.status, time.Since(start))
			}
		})
	}
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
	written bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.written {
		w.status = code
		w.written = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

// NoopMetrics is a no-op metrics collector for when no metrics system is configured.
type NoopMetrics struct{}

func (n *NoopMetrics) RecordRequest(_, _ string, _ int, _ time.Duration) {}

// LogMetrics logs request metrics via a callback.
type LogMetrics struct {
	LogFunc func(method, path string, status int, duration time.Duration)
}

func (l *LogMetrics) RecordRequest(method, path string, status int, duration time.Duration) {
	if l.LogFunc != nil {
		l.LogFunc(method, path, status, duration)
	}
}

// ensure statusWriter satisfies http.Flusher if underlying writer does
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Header satisfies http.ResponseWriter explicitly for clarity.
func (w *statusWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

// StatusCode returns the status code.
func (w *statusWriter) StatusCode() int {
	return w.status
}

// StatusString returns the status code as a string.
func StatusString(code int) string {
	return strconv.Itoa(code)
}
