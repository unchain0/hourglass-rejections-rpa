package webauthn

import (
	"os"
	"runtime"
)

func IsHeadlessEnvironment() bool {
	if runtime.GOOS != "windows" && os.Getenv("DISPLAY") == "" {
		if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_CLIENT") != "" {
			return true
		}

		ciVars := []string{"CI", "CONTINUOUS_INTEGRATION", "BUILD_ID", "TRAVIS", "CIRCLECI",
			"JENKINS_URL", "GITHUB_ACTIONS", "GITLAB_CI", "DRONE", "BUILDKITE"}
		for _, v := range ciVars {
			if os.Getenv(v) != "" {
				return true
			}
		}

		return true
	}

	return false
}

func (tm *TokenManager) HasWebAuthnCredentials() bool {
	if tm.storagePath == "" {
		return false
	}

	storage, err := NewStorage(tm.storagePath)
	if err != nil {
		return false
	}

	creds, err := storage.Load()
	if err != nil {
		return false
	}

	return len(creds.Credentials) > 0
}
