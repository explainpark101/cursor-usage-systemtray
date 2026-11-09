// Package version holds the application version string.
// build.bat injects the value from go.mod via -ldflags -X.
package version

// Version is set at link time from go.mod "// Version: x.y.z".
var Version = "dev"
