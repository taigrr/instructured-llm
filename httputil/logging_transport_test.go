package httputil

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncBuffer is a concurrency-safe io.Writer for capturing slog output.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestLoggingTransport_RedactsSensitiveHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "super-secret-token", r.Header.Get("Authorization"),
			"the real outgoing request must keep its real credentials")
		w.Header().Set("Set-Cookie", "session=abc123")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client := &http.Client{
		Transport: &LoggingTransport{Logger: logger},
	}

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "super-secret-token")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	logged := buf.String()
	assert.NotContains(t, logged, "super-secret-token")
	assert.NotContains(t, logged, "session=abc123")
	assert.Contains(t, logged, "REDACTED")

	// The caller's header map must be left untouched.
	assert.Equal(t, "super-secret-token", req.Header.Get("Authorization"))
}

func TestLoggingTransport_RedactsExtraHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client := DebugHTTPClientSanitized("X-My-Secret")
	client.Transport.(*Transport).Transport.(*LoggingTransport).Logger = logger //nolint:forcetypeassert

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-My-Secret", "sssh")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.NotContains(t, buf.String(), "sssh")
}

func TestLoggingTransport_RedactsQueryParamCredentials(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client := &http.Client{
		Transport: &LoggingTransport{Logger: logger},
	}

	resp, err := client.Get(server.URL + "/v1/generate?key=super-secret-api-key&q=hello")
	require.NoError(t, err)
	defer resp.Body.Close()

	logged := buf.String()
	assert.NotContains(t, logged, "super-secret-api-key")
	assert.Contains(t, logged, "key=REDACTED")
	assert.Contains(t, logged, "q=hello", "non-sensitive query params should be left untouched")
}

func TestLoggingTransport_SkipsDumpWhenDebugDisabled(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	client := &http.Client{
		Transport: &LoggingTransport{Logger: logger},
	}

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "super-secret-token")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Empty(t, buf.String(), "nothing should be dumped/logged when DEBUG level is disabled")
}

func TestJSONDebugTransport_RedactsQueryParamCredentialsInPrintedURL(t *testing.T) {
	// Swaps the process-wide os.Stdout, so this test cannot run in
	// parallel with anything else that reads/writes it.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	client := &http.Client{Transport: &jsonDebugTransport{}}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/generate?key=super-secret-api-key",
		strings.NewReader(`{"q":"hello"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, doErr := client.Do(req)

	os.Stdout = origStdout
	require.NoError(t, w.Close())

	require.NoError(t, doErr)
	defer resp.Body.Close()

	var out bytes.Buffer
	_, err = out.ReadFrom(r)
	require.NoError(t, err)

	printed := out.String()
	assert.NotContains(t, printed, "super-secret-api-key")
	assert.Contains(t, printed, "key=REDACTED")
}
