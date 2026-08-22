package httputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// defaultRedactedHeaders lists header names that are redacted by default
// whenever a request or response is logged or printed, so credentials never
// end up in debug output.
var defaultRedactedHeaders = []string{ //nolint:gochecknoglobals
	"Authorization",
	"Api-Key",
	"X-Api-Key",
	"X-Auth-Token",
	"Proxy-Authorization",
	"Cookie",
	"Set-Cookie",
}

// defaultRedactedQueryParams lists URL query parameter names that are
// redacted by default. Several providers pass credentials as a query
// parameter (e.g. "?key=...") rather than a header, so header redaction
// alone is not enough to keep secrets out of logged request lines/URLs.
var defaultRedactedQueryParams = []string{ //nolint:gochecknoglobals
	"key",
	"api_key",
	"apikey",
	"access_token",
	"token",
	"secret",
}

// LoggingClient is an [http.Client] that logs complete HTTP requests and responses
// using structured logging via [slog]. This client is useful for debugging API
// interactions, as it captures the full request and response including headers
// and bodies. The logs are emitted at DEBUG level.
//
// The headers listed in defaultRedactedHeaders and the query parameters
// listed in defaultRedactedQueryParams are always redacted before logging.
//
// Example:
//
//	slog.SetLogLoggerLevel(slog.LevelDebug)
//	resp, err := httputil.LoggingClient.Get("https://api.example.com/data")
var LoggingClient = &http.Client{ //nolint:gochecknoglobals
	Transport: &Transport{
		Transport: &LoggingTransport{},
	},
}

// JSONDebugClient is an [http.Client] designed for debugging JSON APIs.
// It pretty-prints JSON request and response bodies to stdout with ANSI colors:
// requests are shown in blue, responses in green. This client is intended for
// development and debugging purposes only.
//
// Unlike [LoggingClient], this client writes directly to stdout rather than
// using structured logging. It does not print headers, but it does print the
// request URL; sensitive query parameters (see defaultRedactedQueryParams)
// are redacted from it before printing.
var JSONDebugClient = &http.Client{ //nolint:gochecknoglobals
	Transport: &Transport{
		Transport: &jsonDebugTransport{},
	},
}

// DebugHTTPClient is a deprecated alias for [LoggingClient].
//
// Deprecated: Use [LoggingClient] instead.
var DebugHTTPClient = LoggingClient //nolint:gochecknoglobals

// DebugHTTPColorJSON is a deprecated alias for [JSONDebugClient].
//
// Deprecated: Use [JSONDebugClient] instead.
var DebugHTTPColorJSON = JSONDebugClient //nolint:gochecknoglobals

// DebugHTTPClientSanitized returns an [http.Client] like [LoggingClient] that
// additionally redacts the given header names (on top of the always-redacted
// defaultRedactedHeaders) before logging requests and responses.
var DebugHTTPClientSanitized = func(extraRedactedHeaders ...string) *http.Client { //nolint:gochecknoglobals
	redacted := make([]string, 0, len(defaultRedactedHeaders)+len(extraRedactedHeaders))
	redacted = append(redacted, defaultRedactedHeaders...)
	redacted = append(redacted, extraRedactedHeaders...)
	return &http.Client{
		Transport: &Transport{
			Transport: &LoggingTransport{RedactHeaders: redacted},
		},
	}
}

// LoggingTransport is an [http.RoundTripper] that logs complete HTTP requests
// and responses using structured logging. It's designed for debugging and
// development purposes.
//
// The transport logs at DEBUG level, so ensure your logger is configured
// appropriately to see the output; when DEBUG logging isn't enabled, the
// (relatively expensive) request/response dump is skipped entirely.
type LoggingTransport struct {
	// Transport is the underlying [http.RoundTripper] to use.
	// If nil, [http.DefaultTransport] is used.
	Transport http.RoundTripper

	// Logger is the [slog.Logger] to use for logging.
	// If nil, [slog.Default] is used.
	Logger *slog.Logger

	// RedactHeaders lists additional header names whose values are replaced
	// with "REDACTED" before logging. defaultRedactedHeaders are always
	// redacted regardless of this field.
	RedactHeaders []string
}

// RoundTrip implements the [http.RoundTripper] interface. It logs the complete
// HTTP request (including headers and body) before sending it, executes the
// request using the underlying transport, then logs the complete response.
// Both request and response are logged at DEBUG level.
//
// Redaction is applied to the dumped text only - the live request/response
// objects (including their Header maps) are never mutated, so the real
// outgoing request always keeps its real credentials.
func (t *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	logger := t.Logger
	if logger == nil {
		logger = slog.Default()
	}
	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	debugEnabled := logger.Enabled(req.Context(), slog.LevelDebug)

	if debugEnabled {
		requestDump, err := httputil.DumpRequestOut(req, true)
		if err != nil {
			logger.Error("Failed to dump request", "error", err)
		} else {
			logger.Debug(string(redact(requestDump, t.RedactHeaders)))
		}
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		// Return the underlying error unwrapped: callers (including
		// [http.Client] itself) type-assert on net.Error for timeout/
		// temporary-error handling, which a wrapped error would defeat.
		return nil, err //nolint:wrapcheck
	}

	if debugEnabled {
		responseDump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			logger.Error("Failed to dump response", "error", err)
		} else {
			logger.Debug(string(redact(responseDump, t.RedactHeaders)))
		}
	}
	return resp, nil
}

// redact scans a dumped HTTP request/response (as produced by
// [httputil.DumpRequestOut]/[httputil.DumpResponse]) and replaces the value
// of any sensitive header, and any sensitive query parameter in the request
// line/URL, with "REDACTED". It operates purely on the dumped bytes, so it
// never touches the live request/response that was actually sent.
func redact(dump []byte, extraHeaders []string) []byte {
	if len(dump) == 0 {
		return dump
	}

	redactedNames := make(map[string]bool, len(defaultRedactedHeaders)+len(extraHeaders))
	for _, name := range defaultRedactedHeaders {
		redactedNames[strings.ToLower(name)] = true
	}
	for _, name := range extraHeaders {
		redactedNames[strings.ToLower(name)] = true
	}

	lines := strings.Split(string(dump), "\n")
	inHeaders := true
	for i, line := range lines {
		trimmed := strings.TrimSuffix(line, "\r")
		if trimmed == "" {
			// Blank line marks the end of the header section.
			inHeaders = false
			continue
		}
		if !inHeaders {
			continue
		}
		idx := strings.Index(trimmed, ":")
		if idx <= 0 {
			// Request/status line, not a "Header: value" line.
			continue
		}
		name := strings.ToLower(strings.TrimSpace(trimmed[:idx]))
		if redactedNames[name] {
			suffix := ""
			if strings.HasSuffix(line, "\r") {
				suffix = "\r"
			}
			lines[i] = trimmed[:idx+1] + " REDACTED" + suffix
		}
	}

	return redactQueryParams([]byte(strings.Join(lines, "\n")))
}

var queryParamPatterns = buildQueryParamPatterns(defaultRedactedQueryParams) //nolint:gochecknoglobals

func buildQueryParamPatterns(names []string) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(names))
	for _, name := range names {
		patterns = append(patterns, regexp.MustCompile(`(?i)([?&]`+regexp.QuoteMeta(name)+`=)[^&#\s]*`))
	}
	return patterns
}

// redactQueryParams replaces the value of any known sensitive query
// parameter (see defaultRedactedQueryParams) with "REDACTED".
func redactQueryParams(dump []byte) []byte {
	for _, re := range queryParamPatterns {
		dump = re.ReplaceAll(dump, []byte("${1}REDACTED"))
	}
	return dump
}

// redactURL returns u's string form with sensitive query parameters (see
// defaultRedactedQueryParams) replaced by "REDACTED". Used by jsonDebugTransport,
// which prints the request URL directly rather than a full HTTP dump.
func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	return string(redactQueryParams([]byte(u.String())))
}

// ANSI color codes.
const (
	colorBlue  = "\033[34m"
	colorGreen = "\033[32m"
	colorReset = "\033[0m"
)

// colorize wraps s in the given ANSI color code, unless the NO_COLOR
// environment variable is set (https://no-color.org), in which case it is
// returned unchanged so output piped to a file/CI log stays free of escape
// codes.
func colorize(color, s string) string {
	if os.Getenv("NO_COLOR") != "" {
		return s
	}
	return color + s + colorReset
}

type jsonDebugTransport struct {
	Transport http.RoundTripper
}

// drainAndReplaceBody reads body fully, closes it, and returns the bytes
// read along with a fresh io.ReadCloser the caller should assign back so
// the body can still be consumed downstream. A read error is fatal because
// the body cannot be faithfully forwarded once it is only partially read; a
// close error is not, since the full contents were already captured.
func drainAndReplaceBody(body io.ReadCloser, what string) ([]byte, io.ReadCloser, error) {
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read %s body: %w", what, err)
	}
	return data, io.NopCloser(bytes.NewReader(data)), nil
}

// printPrettyJSON pretty-prints body under the given color-coded label, if
// it parses as JSON. Non-JSON bodies are silently skipped.
func printPrettyJSON(color, label string, body []byte) {
	var pretty bytes.Buffer
	if json.Indent(&pretty, body, "", "  ") == nil {
		fmt.Printf("%s\n%s\n", colorize(color, label), pretty.String()) //nolint:forbidigo
	}
}

func (t *jsonDebugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	if strings.Contains(req.Header.Get("Content-Type"), "application/json") && req.Body != nil {
		body, replacement, err := drainAndReplaceBody(req.Body, "request")
		if err != nil {
			return nil, err
		}
		req.Body = replacement
		printPrettyJSON(colorBlue, "Request to "+redactURL(req.URL), body)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "application/json") && resp.Body != nil {
		body, replacement, err := drainAndReplaceBody(resp.Body, "response")
		if err != nil {
			return nil, err
		}
		resp.Body = replacement
		printPrettyJSON(colorGreen, fmt.Sprintf("Response %d", resp.StatusCode), body)
	}

	return resp, nil
}
