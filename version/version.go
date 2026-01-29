// Package version provides build-time version information for the i2plan mesh VPN.
//
// Version is set at build time using ldflags:
//
//	go build -ldflags "-X github.com/go-i2p/wireguard/version.Version=1.0.0"
//
// For development builds, the default "dev" version is used.
package version

// Version is the software version, set at build time via ldflags.
// Example: go build -ldflags "-X github.com/go-i2p/wireguard/version.Version=1.0.0"
var Version = "dev"

// GitCommit is the git commit hash, set at build time via ldflags.
// Example: go build -ldflags "-X github.com/go-i2p/wireguard/version.GitCommit=$(git rev-parse --short HEAD)"
var GitCommit = ""

// BuildTime is when the binary was built, set at build time via ldflags.
// Example: go build -ldflags "-X github.com/go-i2p/wireguard/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var BuildTime = ""

// Full returns the full version string including commit and build time if available.
func Full() string {
	v := Version
	if GitCommit != "" {
		v += "-" + GitCommit
	}
	if BuildTime != "" {
		v += " (" + BuildTime + ")"
	}
	return v
}
