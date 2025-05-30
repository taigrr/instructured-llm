package httputil

import (
	"fmt"
	"net/http"
	"net/http/httputil"
)

// DebugHTTPClient is an http.Client that logs the request and response with full contents.
var DebugHTTPClient = &http.Client{ //nolint:gochecknoglobals
	Transport: &logTransport{http.DefaultTransport, nil},
}

var DebugHTTPClientSanitized = func(ignoreList ...string) *http.Client { //nolint:gochecknoglobals
	return &http.Client{
		Transport: &logTransport{
			Transport:  http.DefaultTransport,
			ignoreList: ignoreList,
		},
	}
}

type logTransport struct {
	Transport  http.RoundTripper
	ignoreList []string // List of headers to ignore in the dump
}

// RoundTrip logs the request and response with full contents using httputil.DumpRequest and httputil.DumpResponse.
func (t *logTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Remove headers that are in the ignore list
	reqCopy := req.Clone(req.Context())
	for _, header := range t.ignoreList {
		found := reqCopy.Header.Get(header) != ""
		reqCopy.Header.Del(header)
		if found {
			reqCopy.Header.Add(header, "REDACTED")
		}
	}
	dump, err := httputil.DumpRequestOut(reqCopy, true)
	if err != nil {
		return nil, err
	}
	fmt.Println(string(dump)) //nolint:forbidigo
	resp, err := t.Transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// back up original headers before modifying
	respHeaders := resp.Header.Clone()

	// Remove headers that are in the ignore list
	for _, header := range t.ignoreList {
		found := resp.Header.Get(header) != ""
		resp.Header.Del(header)
		if found {
			resp.Header.Add(header, "REDACTED")
		}
	}
	dump, err = httputil.DumpResponse(resp, true)
	if err != nil {
		return nil, err
	}

	fmt.Println(string(dump)) //nolint:forbidigo

	// Restore original headers for the response
	resp.Header = respHeaders
	return resp, nil
}
