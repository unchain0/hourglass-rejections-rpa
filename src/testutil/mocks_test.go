package testutil

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMockFileSystem(t *testing.T) {
	fs := NewMockFileSystem()
	assert.NotNil(t, fs)
	assert.NotNil(t, fs.Files)
	assert.Empty(t, fs.Calls.ReadFile)
}

func TestMockFileSystem_UserHomeDir(t *testing.T) {
	fs := NewMockFileSystem()
	fs.HomeDir = "/home/test"
	fs.HomeDirErr = nil

	home, err := fs.UserHomeDir()
	assert.NoError(t, err)
	assert.Equal(t, "/home/test", home)
}

func TestMockFileSystem_UserHomeDir_Error(t *testing.T) {
	fs := NewMockFileSystem()
	fs.HomeDirErr = errors.New("home dir error")

	home, err := fs.UserHomeDir()
	assert.Error(t, err)
	assert.Empty(t, home)
}

func TestMockFileSystem_ReadFile(t *testing.T) {
	fs := NewMockFileSystem()
	fs.Files["/test/file.txt"] = []byte("file content")

	content, err := fs.ReadFile("/test/file.txt")
	assert.NoError(t, err)
	assert.Equal(t, "file content", string(content))
	assert.Equal(t, []string{"/test/file.txt"}, fs.Calls.ReadFile)
}

func TestMockFileSystem_ReadFile_NotFound(t *testing.T) {
	fs := NewMockFileSystem()

	content, err := fs.ReadFile("/nonexistent/file.txt")
	assert.Error(t, err)
	assert.Nil(t, content)
	assert.Contains(t, err.Error(), "file not found")
}

func TestMockFileSystem_ReadFile_Error(t *testing.T) {
	fs := NewMockFileSystem()
	fs.ReadErr = errors.New("read error")

	content, err := fs.ReadFile("/test/file.txt")
	assert.Error(t, err)
	assert.Nil(t, content)
}

func TestMockFileSystem_WriteFile(t *testing.T) {
	fs := NewMockFileSystem()

	err := fs.WriteFile("/test/file.txt", []byte("data"), 0644)
	assert.NoError(t, err)
	assert.Equal(t, []byte("data"), fs.Files["/test/file.txt"])
	assert.Equal(t, []string{"/test/file.txt"}, fs.Calls.WriteFile)
}

func TestMockFileSystem_WriteFile_Error(t *testing.T) {
	fs := NewMockFileSystem()
	fs.WriteErr = errors.New("write error")

	err := fs.WriteFile("/test/file.txt", []byte("data"), 0644)
	assert.Error(t, err)
}

func TestMockFileSystem_MkdirAll(t *testing.T) {
	fs := NewMockFileSystem()

	err := fs.MkdirAll("/test/dir", 0755)
	assert.NoError(t, err)
	assert.Equal(t, []string{"/test/dir"}, fs.Calls.MkdirAll)
}

func TestMockFileSystem_MkdirAll_Error(t *testing.T) {
	fs := NewMockFileSystem()
	fs.MkdirErr = errors.New("mkdir error")

	err := fs.MkdirAll("/test/dir", 0755)
	assert.Error(t, err)
}

func TestNewMockHTTPClient(t *testing.T) {
	client := NewMockHTTPClient()
	assert.NotNil(t, client)
	assert.NotNil(t, client.Requests)
}

func TestMockHTTPClient_Do(t *testing.T) {
	client := NewMockHTTPClient()
	client.Response = &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(nil),
	}

	req, _ := http.NewRequest("GET", "http://test.com", nil)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Len(t, client.Requests, 1)
}

func TestMockHTTPClient_Do_Error(t *testing.T) {
	client := NewMockHTTPClient()
	client.Err = errors.New("request error")

	req, _ := http.NewRequest("GET", "http://test.com", nil)
	resp, err := client.Do(req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestNewMockUserInput(t *testing.T) {
	ui := NewMockUserInput()
	assert.NotNil(t, ui)
}

func TestMockUserInput_ReadLine(t *testing.T) {
	ui := NewMockUserInput()
	ui.Lines = []string{"user input"}

	line, err := ui.ReadLine()
	assert.NoError(t, err)
	assert.Equal(t, "user input", line)
	assert.Equal(t, 1, ui.Calls.ReadLine)
}

func TestMockUserInput_ReadLine_Error(t *testing.T) {
	ui := NewMockUserInput()
	ui.LineErr = errors.New("read error")

	line, err := ui.ReadLine()
	assert.Error(t, err)
	assert.Empty(t, line)
}

func TestMockUserInput_ReadLine_Multiple(t *testing.T) {
	ui := NewMockUserInput()
	ui.Lines = []string{"line1", "line2", "line3"}

	line1, _ := ui.ReadLine()
	line2, _ := ui.ReadLine()
	line3, _ := ui.ReadLine()

	assert.Equal(t, "line1", line1)
	assert.Equal(t, "line2", line2)
	assert.Equal(t, "line3", line3)
}

func TestMockUserInput_ReadLine_Empty(t *testing.T) {
	ui := NewMockUserInput()
	ui.Lines = []string{}

	line, err := ui.ReadLine()
	assert.Error(t, err)
	assert.Equal(t, io.EOF, err)
	assert.Empty(t, line)
}

func TestMockUserInput_Confirm(t *testing.T) {
	ui := NewMockUserInput()
	ui.Confirmations = []bool{true}

	result, err := ui.Confirm("Are you sure?")
	assert.NoError(t, err)
	assert.True(t, result)
	assert.Equal(t, 1, ui.Calls.Confirm)
	assert.Equal(t, []string{"Are you sure?"}, ui.Calls.ConfirmMsg)
}

func TestMockUserInput_Confirm_False(t *testing.T) {
	ui := NewMockUserInput()
	ui.Confirmations = []bool{false}

	result, err := ui.Confirm("Are you sure?")
	assert.NoError(t, err)
	assert.False(t, result)
}

func TestMockUserInput_Confirm_Error(t *testing.T) {
	ui := NewMockUserInput()
	ui.ConfirmErr = errors.New("confirm error")

	result, err := ui.Confirm("Are you sure?")
	assert.Error(t, err)
	assert.False(t, result)
}

func TestMockUserInput_Confirm_Multiple(t *testing.T) {
	ui := NewMockUserInput()
	ui.Confirmations = []bool{true, false, true}

	result1, _ := ui.Confirm("First?")
	result2, _ := ui.Confirm("Second?")
	result3, _ := ui.Confirm("Third?")

	assert.True(t, result1)
	assert.False(t, result2)
	assert.True(t, result3)
}

func TestMockUserInput_Confirm_Empty(t *testing.T) {
	ui := NewMockUserInput()
	ui.Confirmations = []bool{}

	result, err := ui.Confirm("Are you sure?")
	assert.Error(t, err)
	assert.Equal(t, io.EOF, err)
	assert.False(t, result)
}

func TestNewMockSCPClient(t *testing.T) {
	client := NewMockSCPClient()
	assert.NotNil(t, client)
}

func TestMockSCPClient_CopyFile(t *testing.T) {
	client := NewMockSCPClient()

	err := client.CopyFile("/local/file", "/remote/file")
	assert.NoError(t, err)
	assert.Equal(t, 1, client.Calls)
	assert.Len(t, client.Copies, 1)
	assert.Equal(t, "/local/file", client.Copies[0].Src)
	assert.Equal(t, "/remote/file", client.Copies[0].Dst)
}

func TestMockSCPClient_CopyFile_Error(t *testing.T) {
	client := NewMockSCPClient()
	client.Err = errors.New("scp error")

	err := client.CopyFile("/local/file", "/remote/file")
	assert.Error(t, err)
}

func TestMockJSONResponse(t *testing.T) {
	resp := MockJSONResponse(200, `{"key": "value"}`)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "key")
	assert.Contains(t, string(body), "value")
}

func TestMockTextResponse(t *testing.T) {
	resp := MockTextResponse(200, "hello world")

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"))

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "hello world", string(body))
}

func TestMockErrorResponse(t *testing.T) {
	resp := MockErrorResponse(404, "not found")

	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "not found")
}
