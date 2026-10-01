package main

// version is the product version.
//
// It is defined in this source file. Changing it requires editing this
// declaration. The running binary has no link-time override for it.
const version = "0.1.0-beta.2"

// commit and buildDate are build metadata.
//
// A local build leaves them as unknown. A release build may override them
// with -ldflags "-X main.commit=<commit> -X main.buildDate=<buildDate>".
// Git metadata may be supplied to that build. The running binary does not
// inspect Git or any other version-control system, and it does not infer
// these values from the working directory.
var (
	commit    = "unknown"
	buildDate = "unknown"
)

// versionText is the complete stdout of the version command, including its
// single terminating newline.
func versionText() string {
	return "nodex " + version + "\n" +
		"commit " + commit + "\n" +
		"built " + buildDate + "\n"
}
