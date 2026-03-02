package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
)

var (
	stdout   io.Writer = os.Stdout
	stderr   io.Writer = os.Stderr
	randRead           = rand.Read
	exitFunc           = os.Exit
)

func main() {
	exitCode := run(stdout, stderr)
	exitFunc(exitCode)
}

func generateKey() ([]byte, error) {
	key := make([]byte, 32)
	n, err := randRead(key)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	if n != 32 {
		return nil, fmt.Errorf("failed to generate key: short read %d/32", n)
	}
	return key, nil
}

func formatKey(key []byte) string {
	return fmt.Sprintf("DB_ENCRYPTION_KEY=%x", key)
}

func run(stdout, stderr io.Writer) int {
	key, err := generateKey()
	if err != nil {
		fmt.Fprintf(stderr, "Failed to generate key: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "Generated encryption key:")
	fmt.Fprintf(stdout, "%s\n\n", formatKey(key))
	fmt.Fprintln(stdout, "Add this to your .env file to enable database encryption.")
	fmt.Fprintln(stdout, "WARNING: Keep this key secure and never commit it!")

	return 0
}
