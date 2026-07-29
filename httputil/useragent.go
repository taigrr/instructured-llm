package httputil

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

// langchaingoModulePath is this library's own module path, used to look up
// its version in build info and to avoid double-reporting it as both the
// "program" and the library itself when a binary is built directly from
// this module (e.g. running `go test ./...` inside this repo).
const langchaingoModulePath = "github.com/tmc/langchaingo"

var (
	userAgent     string
	userAgentOnce sync.Once
)

// UserAgent returns the default User-Agent string for this library's HTTP
// clients.
//
// Format: program/version langchaingo/version Go/version (GOOS GOARCH)
// Example: "openai-chat-example/devel langchaingo/v0.1.8 Go/go1.21.0 (darwin arm64)"
func UserAgent() string {
	userAgentOnce.Do(func() {
		parts := []string{}

		if info, ok := debug.ReadBuildInfo(); ok {
			langchainVer := "devel"

			programName := ""
			programVersion := "devel"
			if info.Main.Path != "" && info.Main.Path != "command-line-arguments" {
				programName = info.Main.Path[strings.LastIndex(info.Main.Path, "/")+1:]
				if info.Main.Version != "" && info.Main.Version != "(devel)" {
					programVersion = info.Main.Version
				}
			}

			// Skip the redundant "program" segment only when the running
			// binary *is* this module itself (e.g. its own test binary),
			// where it would otherwise duplicate the "langchaingo/<version>"
			// segment below.
			if programName != "" && info.Main.Path != langchaingoModulePath {
				parts = append(parts, programName+"/"+programVersion)
			}

			if info.Main.Path == langchaingoModulePath {
				langchainVer = programVersion
			} else {
				for _, dep := range info.Deps {
					if dep.Path == langchaingoModulePath {
						if v := strings.Trim(dep.Version, "()"); v != "" {
							langchainVer = v
						}
						break
					}
				}
			}
			parts = append(parts, "langchaingo/"+langchainVer)
		} else {
			parts = append(parts, "langchaingo/devel")
		}

		parts = append(parts, fmt.Sprintf("Go/%s", runtime.Version()))
		parts = append(parts, fmt.Sprintf("(%s %s)", runtime.GOOS, runtime.GOARCH))

		userAgent = strings.Join(parts, " ")
	})
	return userAgent
}
