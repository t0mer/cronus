// Package version exposes build-time version information for Cronus.
package version

import "runtime/debug"

// Version is the build version string, injected at build time via
// -ldflags "-X github.com/t0mer/cronus/internal/version.Version=<v>".
// It falls back to VCS info or "dev" when built without ldflags.
var Version = ""

// String returns the resolved version string. When Version was not injected
// at build time it attempts to derive one from the embedded build info, and
// otherwise reports "dev".
func String() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return "dev"
}
