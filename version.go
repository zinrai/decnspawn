package main

import "fmt"

// Build information injected at release time via goreleaser ldflags
// (see .goreleaser.yaml). Defaults apply to a local `go build`.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func printVersion() {
	fmt.Printf("decnspawn %s\n", version)
	fmt.Printf("commit: %s\n", commit)
	fmt.Printf("built:  %s\n", date)
}
