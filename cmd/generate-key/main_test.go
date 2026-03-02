package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestGenerateKey_Success(t *testing.T) {
	key, err := generateKey()
	if err != nil {
		t.Fatalf("generateKey() error = %v", err)
	}

	if len(key) != 32 {
		t.Errorf("generateKey() returned %d bytes, want 32", len(key))
	}
}

func TestGenerateKey_Unique(t *testing.T) {
	key1, err := generateKey()
	if err != nil {
		t.Fatalf("generateKey() error = %v", err)
	}

	key2, err := generateKey()
	if err != nil {
		t.Fatalf("generateKey() error = %v", err)
	}

	if bytes.Equal(key1, key2) {
		t.Error("generateKey() returned identical keys - should be random")
	}
}

func TestGenerateKey_RandReadError(t *testing.T) {
	originalRandRead := randRead
	defer func() { randRead = originalRandRead }()

	randRead = func(b []byte) (n int, err error) {
		return 0, errors.New("random source failed")
	}

	_, err := generateKey()
	if err == nil {
		t.Error("generateKey() should return error when rand.Read fails")
	}

	if !strings.Contains(err.Error(), "failed to generate key") {
		t.Errorf("error message should contain 'failed to generate key', got: %v", err)
	}
}

func TestGenerateKey_ShortRead(t *testing.T) {
	originalRandRead := randRead
	defer func() { randRead = originalRandRead }()

	randRead = func(b []byte) (n int, err error) {
		return 16, nil
	}

	_, err := generateKey()
	if err == nil {
		t.Error("generateKey() should return error on short read")
	}
}

func TestFormatKey(t *testing.T) {
	key := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}

	result := formatKey(key)

	expected := "DB_ENCRYPTION_KEY=0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	if result != expected {
		t.Errorf("formatKey() = %q, want %q", result, expected)
	}
}

func TestFormatKey_Empty(t *testing.T) {
	result := formatKey([]byte{})
	expected := "DB_ENCRYPTION_KEY="
	if result != expected {
		t.Errorf("formatKey() = %q, want %q", result, expected)
	}
}

func TestRun_Success(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run(stdout, stderr)

	if exitCode != 0 {
		t.Errorf("run() exit code = %d, want 0", exitCode)
	}

	output := stdout.String()
	if !strings.Contains(output, "Generated encryption key:") {
		t.Error("output should contain 'Generated encryption key:'")
	}

	if !strings.Contains(output, "DB_ENCRYPTION_KEY=") {
		t.Error("output should contain 'DB_ENCRYPTION_KEY='")
	}

	if !strings.Contains(output, "Add this to your .env file") {
		t.Error("output should contain setup instructions")
	}

	if !strings.Contains(output, "WARNING: Keep this key secure") {
		t.Error("output should contain security warning")
	}
}

func TestRun_Error(t *testing.T) {
	originalRandRead := randRead
	defer func() { randRead = originalRandRead }()

	randRead = func(b []byte) (n int, err error) {
		return 0, errors.New("entropy exhausted")
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run(stdout, stderr)

	if exitCode != 1 {
		t.Errorf("run() exit code = %d, want 1", exitCode)
	}

	errOutput := stderr.String()
	if !strings.Contains(errOutput, "Failed to generate key") {
		t.Error("stderr should contain error message")
	}

	if !strings.Contains(errOutput, "entropy exhausted") {
		t.Error("stderr should contain original error")
	}
}

func TestRun_KeyLength(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run(stdout, stderr)

	if exitCode != 0 {
		t.Fatalf("run() failed with exit code %d", exitCode)
	}

	output := stdout.String()
	lines := strings.Split(output, "\n")

	var keyLine string
	for _, line := range lines {
		if strings.HasPrefix(line, "DB_ENCRYPTION_KEY=") {
			keyLine = line
			break
		}
	}

	if keyLine == "" {
		t.Fatal("output should contain DB_ENCRYPTION_KEY line")
	}

	keyHex := strings.TrimPrefix(keyLine, "DB_ENCRYPTION_KEY=")
	if len(keyHex) != 64 {
		t.Errorf("hex key length = %d, want 64 (32 bytes * 2)", len(keyHex))
	}
}
