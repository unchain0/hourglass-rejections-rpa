package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"hourglass-rejections-rpa/internal/auth/webauthn"
)

func TestNewRegistrationRunner(t *testing.T) {
	runner := newRegistrationRunner()
	assert.NotNil(t, runner)
	assert.NotNil(t, runner.userHomeDir)
	assert.NotNil(t, runner.mkdirAll)
	assert.NotNil(t, runner.consoleInput)
	assert.NotNil(t, runner.confirm)
}

func TestRegistrationRunner_getUsername_InvalidEmail(t *testing.T) {
	inputIdx := 0
	inputs := []string{"notanemail\n", "yes\n", "test@example.com\n"}
	runner := &registrationRunner{
		consoleInput: func(prompt string) (string, error) {
			if inputIdx < len(inputs) {
				input := inputs[inputIdx]
				inputIdx++
				return input, nil
			}
			return "", errors.New("no more inputs")
		},
		confirm: func(prompt string) (bool, error) {
			return true, nil
		},
	}
	username, err := runner.getUsername()
	assert.NoError(t, err)
	assert.Equal(t, "test@example.com", username)
}

func TestRegistrationRunner_getUsername_ConfirmNo(t *testing.T) {
	inputIdx := 0
	inputs := []string{"notanemail\n", "no\n", "test@example.com\n"}
	confirmIdx := 0
	runner := &registrationRunner{
		consoleInput: func(prompt string) (string, error) {
			if inputIdx < len(inputs) {
				input := inputs[inputIdx]
				inputIdx++
				return input, nil
			}
			return "", errors.New("no more inputs")
		},
		confirm: func(prompt string) (bool, error) {
			confirmIdx++
			if confirmIdx == 1 {
				return false, nil
			}
			return true, nil
		},
	}
	username, err := runner.getUsername()
	assert.NoError(t, err)
	assert.Equal(t, "test@example.com", username)
}

func TestRegistrationRunner_getUsername_Error(t *testing.T) {
	runner := &registrationRunner{
		consoleInput: func(prompt string) (string, error) {
			return "", errors.New("input error")
		},
		confirm: func(prompt string) (bool, error) {
			return true, nil
		},
	}
	_, err := runner.getUsername()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input error")
}

func TestRegistrationRunner_run_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	credsPath := filepath.Join(tmpDir, ".hourglass-rpa", "webauthn-credentials.json")
	os.MkdirAll(filepath.Dir(credsPath), 0o700)
	os.WriteFile(credsPath, []byte("{}"), 0o600)

	inputIdx := 0
	inputs := []string{"yes\n", "test@example.com\n"}

	runner := &registrationRunner{
		userHomeDir: func() (string, error) {
			return tmpDir, nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return os.MkdirAll(path, perm)
		},
		consoleInput: func(prompt string) (string, error) {
			if inputIdx < len(inputs) {
				input := inputs[inputIdx]
				inputIdx++
				return input, nil
			}
			return "", errors.New("no more inputs")
		},
		confirm: func(prompt string) (bool, error) {
			return true, nil
		},
	}

	err := runner.run()
	assert.Error(t, err)
}

func TestRegistrationRunner_run_KeepExisting(t *testing.T) {
	tmpDir := t.TempDir()
	credsPath := filepath.Join(tmpDir, ".hourglass-rpa", "webauthn-credentials.json")
	os.MkdirAll(filepath.Dir(credsPath), 0o700)
	os.WriteFile(credsPath, []byte("{}"), 0o600)

	runner := &registrationRunner{
		userHomeDir: func() (string, error) {
			return tmpDir, nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return os.MkdirAll(path, perm)
		},
		consoleInput: func(prompt string) (string, error) {
			return "no\n", nil
		},
		confirm: func(prompt string) (bool, error) {
			return false, nil
		},
	}

	err := runner.run()
	assert.NoError(t, err)
}

func TestRegistrationRunner_run_MkdirError(t *testing.T) {
	runner := &registrationRunner{
		userHomeDir: func() (string, error) {
			return "/tmp/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return errors.New("mkdir error")
		},
		consoleInput: func(prompt string) (string, error) {
			return "test", nil
		},
		confirm: func(prompt string) (bool, error) {
			return true, nil
		},
	}
	err := runner.run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mkdir error")
}

func TestRegistrationRunner_getUsername(t *testing.T) {
	tests := []struct {
		name   string
		inputs []string
		want   string
	}{
		{
			name:   "valid email",
			inputs: []string{"test@example.com\n"},
			want:   "test@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputIdx := 0
			runner := &registrationRunner{
				consoleInput: func(prompt string) (string, error) {
					if inputIdx < len(tt.inputs) {
						input := tt.inputs[inputIdx]
						inputIdx++
						return input, nil
					}
					return "", errors.New("no more inputs")
				},
				confirm: func(prompt string) (bool, error) {
					return true, nil
				},
			}
			assert.NotNil(t, runner)
		})
	}
}

func TestRegistrationRunner_printSuccess(t *testing.T) {
	runner := newRegistrationRunner()
	creds := &webauthn.Credential{
		ID:       "test-credential-id-12345",
		UserName: "test@example.com",
	}
	credsPath := "/tmp/test-credentials.json"
	runner.printSuccess(creds, credsPath)
}

func TestMainFunc(t *testing.T) {
	origOsExit := osExit
	defer func() { osExit = origOsExit }()

	var exitCode int
	osExit = func(code int) {
		exitCode = code
		panic("exit called")
	}

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	assert.Panics(t, func() {
		main()
	})
	assert.Equal(t, 1, exitCode)
}
