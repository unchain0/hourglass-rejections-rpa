//go:build !verify

package main

import "log"

// Stub so `go test ./...` without -tags verify does not see an empty package.
// launch.sh always builds with -tags verify.
func main() {
	log.Fatal("verify-harness requires: go build -tags verify")
}
