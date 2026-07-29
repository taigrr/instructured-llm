package httputil

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserAgent(t *testing.T) {
	t.Parallel()

	ua := UserAgent()

	assert.NotEmpty(t, ua)
	assert.Contains(t, ua, "langchaingo/")
	assert.Contains(t, ua, "Go/"+runtime.Version())
	assert.Contains(t, ua, runtime.GOOS)
	assert.Contains(t, ua, runtime.GOARCH)
	assert.Contains(t, ua, "("+runtime.GOOS+" "+runtime.GOARCH+")")
	assert.NotContains(t, ua, "  ")

	// Verify subsequent calls return the same (cached) value.
	ua2 := UserAgent()
	assert.Equal(t, ua, ua2)
}

func TestUserAgentFormat(t *testing.T) {
	t.Parallel()

	ua := UserAgent()

	assert.Contains(t, ua, "langchaingo/")
	assert.Contains(t, ua, "Go/")
	assert.Regexp(t, `Go/\S+`, ua, "should contain a Go version token (runtime.Version() format varies across toolchains)")
	assert.Regexp(t, `\([a-z0-9]+ [a-z0-9_]+\)$`, ua, "should end with '(OS ARCH)' format")
}

func TestUserAgentDoesNotDuplicateLangchaingoSegment(t *testing.T) {
	t.Parallel()

	// Running `go test` inside this very module means info.Main.Path is
	// "github.com/tmc/langchaingo" itself, which previously caused the
	// program-name segment and the library segment to collide, producing
	// "langchaingo/devel langchaingo/devel ...".
	ua := UserAgent()
	assert.Equal(t, 1, strings.Count(ua, "langchaingo/"),
		"the langchaingo/<version> segment should only appear once: %q", ua)
}
