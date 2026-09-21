package logging

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

var httpRequestSequence atomic.Uint64

// HTTPMiddleware records every local Core HTTP interface without logging
// query values, headers, or bodies, which may contain SSH credentials.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestID := httpRequestSequence.Add(1)
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: response, status: http.StatusOK}
		path := request.URL.Path
		queryKeys := make([]string, 0, len(request.URL.Query()))
		for key := range request.URL.Query() {
			queryKeys = append(queryKeys, key)
		}
		sort.Strings(queryKeys)
		fields := []any{
			"request_id", requestID,
			"method", request.Method,
			"path", path,
			"query_keys", strings.Join(queryKeys, ","),
			"remote", request.RemoteAddr,
		}
		slog.Debug("Core HTTP request started", fields...)
		next.ServeHTTP(recorder, request)
		fields = append(fields,
			"status", recorder.status,
			"response_bytes", recorder.bytes,
			"duration_ms", time.Since(started).Milliseconds(),
		)
		switch {
		case recorder.status >= http.StatusInternalServerError:
			slog.Error("Core HTTP request completed", fields...)
		case recorder.status >= http.StatusBadRequest:
			slog.Warn("Core HTTP request completed", fields...)
		default:
			slog.Debug("Core HTTP request completed", fields...)
		}
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (recorder *responseRecorder) Unwrap() http.ResponseWriter { return recorder.ResponseWriter }

func (recorder *responseRecorder) WriteHeader(status int) {
	if recorder.wroteHeader {
		return
	}
	recorder.wroteHeader = true
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *responseRecorder) Write(data []byte) (int, error) {
	if !recorder.wroteHeader {
		recorder.WriteHeader(http.StatusOK)
	}
	written, err := recorder.ResponseWriter.Write(data)
	recorder.bytes += int64(written)
	return written, err
}

func (recorder *responseRecorder) ReadFrom(reader io.Reader) (int64, error) {
	if source, ok := recorder.ResponseWriter.(io.ReaderFrom); ok {
		written, err := source.ReadFrom(reader)
		recorder.bytes += written
		return written, err
	}
	return io.Copy(struct{ io.Writer }{recorder}, reader)
}

func (recorder *responseRecorder) Flush() {
	if flusher, ok := recorder.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (recorder *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := recorder.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("HTTP connection does not support hijacking")
	}
	return hijacker.Hijack()
}

func (recorder *responseRecorder) Push(target string, options *http.PushOptions) error {
	if pusher, ok := recorder.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, options)
	}
	return http.ErrNotSupported
}
