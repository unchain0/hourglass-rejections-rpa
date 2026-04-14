### Task 3: Configuração via Environment Variables
- Nenhuma questão encontrada.

- 2026-03-03: Verification blocker unrelated to this change: go test ./internal/auth/webauthn/... fails in webauthn_test.go due to BeginAuthenticationResponse fields Challenge/RpID not matching current type.
- 2026-03-03: `docker build` verification blocked in this environment by DNS timeout resolving `registry-1.docker.io` (temporary network access issue), so image build/headless runtime smoke test could not complete here.
- 2026-03-03: Build verification succeeded after retrying with legacy builder (`DOCKER_BUILDKIT=0`); container includes working `chromium-browser` in headless mode (DBus warnings are non-fatal in minimal containers).
