package httputil

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

const (
	// langchaingoModulePath is this library's own module path, used to look
	// up its version in build info and to avoid double-reporting it as both
	// the "program" and the library itself when a binary is built directly
	// from this module (e.g. running `go test ./...` inside this repo).
	langchaingoModulePath = "github.com/tmc/langchaingo"

	unknownVersion = "devel"
)

var (
	userAgent     string    //nolint:gochecknoglobals
	userAgentOnce sync.Once //nolint:gochecknoglobals
)

// programInfo returns the running binary's program name and version (e.g.
// "openai-chat-example", "devel"), and whether the binary's main module *is*
// this library itself (e.g. its own test binary).
func programInfo(info *debug.BuildInfo) (string, string, bool) {
	if info.Main.Path == "" || info.Main.Path == "command-line-arguments" {
		return "", unknownVersion, false
	}
	name := info.Main.Path[strings.LastIndex(info.Main.Path, "/")+1:]
	version := unknownVersion
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	return name, version, info.Main.Path == langchaingoModulePath
}

// langchaingoVersion returns the version of this library as reported in
// build info: the program's own version when the running binary *is* this
// module, otherwise the version of the github.com/tmc/langchaingo
// dependency (or unknownVersion if it can't be determined).
func langchaingoVersion(info *debug.BuildInfo, programVersion string, isLangchaingo bool) string {
	if isLangchaingo {
		return programVersion
	}
	for _, dep := range info.Deps {
		if dep.Path == langchaingoModulePath {
			if v := strings.Trim(dep.Version, "()"); v != "" {
				return v
			}
			break
		}
	}
	return unknownVersion
}

// UserAgent returns the default User-Agent string for this library's HTTP
// clients.
//
// Format: program/version langchaingo/version Go/version (GOOS GOARCH).
// Example: "openai-chat-example/devel langchaingo/v0.1.8 Go/go1.21.0 (darwin arm64)".
func UserAgent() string {
	userAgentOnce.Do(func() {
		parts := make([]string, 0, 4)

		info, ok := debug.ReadBuildInfo()
		if !ok {
			parts = append(parts, "langchaingo/"+unknownVersion)
		} else {
			programName, programVersion, isLangchaingo := programInfo(info)
			// Skip the redundant "program" segment when the running binary
			// *is* this module itself, where it would otherwise duplicate
			// the "langchaingo/<version>" segment below.
			if programName != "" && !isLangchaingo {
				parts = append(parts, programName+"/"+programVersion)
			}
			parts = append(parts, "langchaingo/"+langchaingoVersion(info, programVersion, isLangchaingo))
		}

		parts = append(parts, fmt.Sprintf("Go/%s", runtime.Version()))
		parts = append(parts, fmt.Sprintf("(%s %s)", runtime.GOOS, runtime.GOARCH))

		userAgent = strings.Join(parts, " ")
	})
	return userAgent
}
