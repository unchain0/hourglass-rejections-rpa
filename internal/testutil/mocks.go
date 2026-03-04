package testutil

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type MockFileSystem struct {
	HomeDir    string
	HomeDirErr error

	Files    map[string][]byte
	ReadErr  error
	WriteErr error
	MkdirErr error

	Calls struct {
		ReadFile  []string
		WriteFile []string
		MkdirAll  []string
	}
}

func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		Files: make(map[string][]byte),
		Calls: struct {
			ReadFile  []string
			WriteFile []string
			MkdirAll  []string
		}{
			ReadFile:  []string{},
			WriteFile: []string{},
			MkdirAll:  []string{},
		},
	}
}

func (m *MockFileSystem) UserHomeDir() (string, error) {
	return m.HomeDir, m.HomeDirErr
}

func (m *MockFileSystem) ReadFile(filename string) ([]byte, error) {
	m.Calls.ReadFile = append(m.Calls.ReadFile, filename)
	if m.ReadErr != nil {
		return nil, m.ReadErr
	}
	if data, exists := m.Files[filename]; exists {
		return data, nil
	}
	return nil, fmt.Errorf("file not found: %s", filename)
}

func (m *MockFileSystem) WriteFile(filename string, data []byte, perm os.FileMode) error {
	m.Calls.WriteFile = append(m.Calls.WriteFile, filename)
	if m.WriteErr != nil {
		return m.WriteErr
	}
	m.Files[filename] = data
	return nil
}

func (m *MockFileSystem) MkdirAll(path string, perm os.FileMode) error {
	m.Calls.MkdirAll = append(m.Calls.MkdirAll, path)
	return m.MkdirErr
}

// MockHTTPClient is a mock implementation of HTTP client.
type MockHTTPClient struct {
	Response *http.Response
	Err      error
	Requests []*http.Request
}

func NewMockHTTPClient() *MockHTTPClient {
	return &MockHTTPClient{
		Requests: []*http.Request{},
	}
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.Requests = append(m.Requests, req)
	return m.Response, m.Err
}

// MockUserInput is a mock implementation for user input operations.
type MockUserInput struct {
	Lines   []string
	LineErr error

	Confirmations []bool
	ConfirmErr    error

	Calls struct {
		ReadLine   int
		Confirm    int
		ConfirmMsg []string
	}
}

func NewMockUserInput() *MockUserInput {
	return &MockUserInput{
		Calls: struct {
			ReadLine   int
			Confirm    int
			ConfirmMsg []string
		}{
			ReadLine:   0,
			Confirm:    0,
			ConfirmMsg: []string{},
		},
	}
}

func (m *MockUserInput) ReadLine() (string, error) {
	m.Calls.ReadLine++
	if m.LineErr != nil {
		return "", m.LineErr
	}
	if len(m.Lines) == 0 {
		return "", io.EOF
	}
	line := m.Lines[0]
	m.Lines = m.Lines[1:]
	return line, nil
}

func (m *MockUserInput) Confirm(msg string) (bool, error) {
	m.Calls.Confirm++
	m.Calls.ConfirmMsg = append(m.Calls.ConfirmMsg, msg)
	if m.ConfirmErr != nil {
		return false, m.ConfirmErr
	}
	if len(m.Confirmations) == 0 {
		return false, io.EOF
	}
	confirm := m.Confirmations[0]
	m.Confirmations = m.Confirmations[1:]
	return confirm, nil
}

// MockSCPClient is a mock implementation for SCP operations.
type MockSCPClient struct {
	Copies []struct {
		Src string
		Dst string
	}
	Err   error
	Calls int
}

func NewMockSCPClient() *MockSCPClient {
	return &MockSCPClient{
		Copies: []struct {
			Src string
			Dst string
		}{},
	}
}

func (m *MockSCPClient) CopyFile(src, dst string) error {
	m.Calls++
	m.Copies = append(m.Copies, struct {
		Src string
		Dst string
	}{src, dst})
	return m.Err
}

// Helper functions for mock HTTP responses.

// MockJSONResponse creates a mock HTTP response with JSON body.
func MockJSONResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

// MockTextResponse creates a mock HTTP response with plain text body.
func MockTextResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
	}
}

// MockErrorResponse creates a mock HTTP error response.
func MockErrorResponse(statusCode int, message string) *http.Response {
	body := fmt.Sprintf(`{"error": "%s"}`, message)
	return MockJSONResponse(statusCode, body)
}
