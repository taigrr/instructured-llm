package duckduckgo

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeRoundTripper returns a canned DuckDuckGo HTML results page for every
// request and records how many times it was invoked, so tests can verify a
// custom HTTP client passed via WithHTTPClient is actually used.
type fakeRoundTripper struct {
	calls int
	html  string
}

func (f *fakeRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	f.calls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(f.html)),
		Header:     make(http.Header),
	}, nil
}

const fakeResultsHTML = `
<div class="web-result">
  <a rel="nofollow" class="result__a" href="/l/?kh=-1&amp;uddg=https%3A%2F%2Fgolang.org">The Go Programming Language</a>
  <a class="result__snippet">Go is an open source programming language.</a>
</div>
`

func TestDuckDuckGoTool_WithHTTPClient(t *testing.T) {
	t.Parallel()

	rt := &fakeRoundTripper{html: fakeResultsHTML}
	tool, err := New(3, DefaultUserAgent, WithHTTPClient(&http.Client{Transport: rt}))
	require.NoError(t, err)

	result, err := tool.Call(context.Background(), "golang programming language")
	require.NoError(t, err)
	require.NotEmpty(t, result)
	require.Equal(t, 1, rt.calls, "the custom HTTP client should have been used for the search request")
	require.Contains(t, result, "Title:")
	require.Contains(t, result, "Go Programming Language")
	require.Contains(t, result, "URL: https://golang.org", "the redirect URL should be uddg-unescaped")
}

func TestDuckDuckGoToolBasicConstruction(t *testing.T) {
	t.Parallel()

	tool, err := New(5, DefaultUserAgent)
	require.NoError(t, err)
	require.NotNil(t, tool)
	require.Equal(t, "DuckDuckGo Search", tool.Name())
	require.NotEmpty(t, tool.Description())
}

func TestDuckDuckGoTool_WithHTTPClientNilIsNoOp(t *testing.T) {
	t.Parallel()

	rt := &fakeRoundTripper{html: fakeResultsHTML}
	tool, err := New(3, DefaultUserAgent, WithHTTPClient(&http.Client{Transport: rt}), WithHTTPClient(nil))
	require.NoError(t, err)

	// A nil client passed to WithHTTPClient must not clobber the
	// previously configured one and must not panic on use.
	_, err = tool.Call(context.Background(), "golang")
	require.NoError(t, err)
	require.Equal(t, 1, rt.calls)
}
