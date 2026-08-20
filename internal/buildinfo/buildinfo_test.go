package buildinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	originalVersion, originalCommit, originalBuildTime := Version, Commit, BuildTime
	t.Cleanup(func() {
		Version, Commit, BuildTime = originalVersion, originalCommit, originalBuildTime
	})
	Version = "1.2.3"
	Commit = "abc123"
	BuildTime = "2026-08-20T12:00:00Z"

	result := String()
	for _, expected := range []string{
		"geo-debug-server 1.2.3",
		"commit: abc123",
		"built: 2026-08-20T12:00:00Z",
		"go: " + runtime.Version(),
		"platform: " + runtime.GOOS + "/" + runtime.GOARCH,
	} {
		if !strings.Contains(result, expected) {
			t.Fatalf("version output does not contain %q:\n%s", expected, result)
		}
	}
}
