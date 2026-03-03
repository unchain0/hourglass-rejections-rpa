
# WebAuthn ECDSA Signature Format - RAW vs ASN.1 DER

## Context (2026-03-03)
Modified `internal/auth/webauthn/authentication.go` to fix WebAuthn authentication with Hourglass server.

## Problem
Server rejected ASN.1 DER format signatures (Go's `asn1.Marshal()` default format). WebAuthn expects RAW (concatenated) format.

## Solution
Changed `encodeECDSASignature()` to produce RAW format (64 bytes):
- Pad `r` to 32 bytes (big-endian)
- Pad `s` to 32 bytes (big-endian)
- Concatenate: `r || s` = 64 bytes total

## Code Change
**Before (ASN.1 DER - variable length):**
```go
func encodeECDSASignature(r, s *big.Int) ([]byte, error) {
    return asn1.Marshal(struct { R, S *big.Int }{R: r, S: s})
}
```

**After (RAW - fixed 64 bytes):**
```go
func encodeECDSASignature(r, s *big.Int) ([]byte, error) {
    signature := make([]byte, 64)
    
    // Pad r to 32 bytes (big-endian)
    rBytes := r.Bytes()
    copy(signature[32-len(rBytes):], rBytes)
    
    // Pad s to 32 bytes (big-endian)
    sBytes := s.Bytes()
    copy(signature[64-len(sBytes):], sBytes)
    
    return signature, nil
}
```

## Key Details
- **Format:** RAW (concatenated) not ASN.1 DER
- **Length:** Exactly 64 bytes (32 + 32)
- **Padding:** Big-endian, left-padded with zeros
- **Import removal:** Removed unused `encoding/asn1` package

## Verification
- ✅ Build passes: `go build ./internal/auth/webauthn/...`
- ✅ No diagnostics errors
- ✅ Security-related comments justified for signature format logic


## Browser WebAuthn Flow (2026-03-03)
Updated `internal/auth/webauthn/browser_auth.go` to replace placeholder sleep-based login with a full browser flow:
- Added retries (`maxAuthAttempts=3`) with transient error detection
- Navigates to `/v2/page/app`, waits for DOM readiness, and polls authentication state
- Detects login/WebAuthn UI signals, attempts one login trigger click, and waits for auth cookies
- Extracts `hglogin` and `X-Hourglass-XSRF-Token` only after successful browser state
- Uses timeout-scoped context and wraps all errors with operation context

Verification:
- `go build ./...` passed
- LSP diagnostics run on changed file (workspace warning only, no file-specific code diagnostics)
