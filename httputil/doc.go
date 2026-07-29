// Package httputil provides HTTP transport and client utilities for instructured-llm.
//
// The package offers several key features:
//
// # User-Agent Management
//
// All HTTP clients and transports in this package automatically add a User-Agent
// header that identifies the library, the calling program, and system
// information. This helps API providers understand client usage patterns and
// aids in debugging.
//
// The User-Agent format is:
//
//	program/version langchaingo/version Go/version (GOOS GOARCH)
//
// For example:
//
//	openai-chat-example/devel langchaingo/v0.1.8 Go/go1.21.0 (darwin arm64)
//
// # Default HTTP Client
//
// The package provides DefaultClient, which is a pre-configured http.Client
// that includes the User-Agent header:
//
//	resp, err := httputil.DefaultClient.Get("https://api.example.com/data")
//
// # Logging and Debugging
//
// For development and debugging, the package provides logging clients. By
// default, common credential-bearing headers (Authorization, API keys,
// cookies, ...) are redacted before anything is logged or printed, so secrets
// never end up in debug output:
//
//	// LoggingClient logs full HTTP requests and responses using slog
//	client := httputil.LoggingClient
//
//	// JSONDebugClient pretty-prints JSON payloads with ANSI colors
//	client := httputil.JSONDebugClient
//
// Additional headers can be redacted with DebugHTTPClientSanitized:
//
//	client := httputil.DebugHTTPClientSanitized("X-My-Secret-Header")
//
// # Custom Transports
//
// The Transport type implements http.RoundTripper and can be used to add
// the library's User-Agent to any HTTP client:
//
//	client := &http.Client{
//	    Transport: &httputil.Transport{
//	        Transport: myCustomTransport,
//	    },
//	}
package httputil
