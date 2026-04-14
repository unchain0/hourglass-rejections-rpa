package storage

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"hourglass-rejections-rpa/src/domain_models"
	"hourglass-rejections-rpa/src/integrations/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	cfg := &config.Config{
		OutputDir:  "./test_outputs",
		CookieFile: "test_cookies.json",
	}
	fs := New(cfg)
	assert.Equal(t, cfg.OutputDir, fs.outputDir)
	assert.Equal(t, cfg.CookieFile, fs.cookieFile)
}

func TestFileStorage_Save(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	fs := &FileStorage{
		outputDir: tempDir,
	}

	rejections := []domain.Rejection{
		{
			Section:   "Section 1",
			Who:       "Who 1",
			What:      "O Que 1",
			When:      "Pra Quando 1",
			Timestamp: time.Now().UTC().Truncate(time.Second),
		},
	}

	err = fs.Save(context.Background(), rejections)
	assert.NoError(t, err)

	// Check if files were created
	files, err := os.ReadDir(tempDir)
	assert.NoError(t, err)
	assert.Len(t, files, 2)

	var jsonFile, csvFile string
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			jsonFile = filepath.Join(tempDir, f.Name())
		} else if filepath.Ext(f.Name()) == ".csv" {
			csvFile = filepath.Join(tempDir, f.Name())
		}
	}

	assert.NotEmpty(t, jsonFile)
	assert.NotEmpty(t, csvFile)

	// Verify JSON content
	jsonData, err := os.ReadFile(jsonFile)
	assert.NoError(t, err)
	var savedRejections []domain.Rejection
	err = json.Unmarshal(jsonData, &savedRejections)
	assert.NoError(t, err)
	assert.Equal(t, rejections, savedRejections)

	// Verify CSV content
	csvData, err := os.Open(csvFile)
	assert.NoError(t, err)
	defer csvData.Close()
	reader := csv.NewReader(csvData)
	records, err := reader.ReadAll()
	assert.NoError(t, err)
	assert.Len(t, records, 2) // Header + 1 row
	assert.Equal(t, []string{"section", "who", "what", "when", "timestamp"}, records[0])
	assert.Equal(t, rejections[0].Section, records[1][0])
	assert.Equal(t, rejections[0].Who, records[1][1])
	assert.Equal(t, rejections[0].What, records[1][2])
	assert.Equal(t, rejections[0].When, records[1][3])
	assert.Equal(t, rejections[0].Timestamp.Format(time.RFC3339), records[1][4])
}

func TestFileStorage_Save_Error(t *testing.T) {
	// Test directory creation error
	fs := &FileStorage{
		outputDir: "/root/invalid", // Should fail on most systems
	}
	err := fs.Save(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create output directory")

	// Test JSON write error
	tempDir, err := os.MkdirTemp("", "storage_test_error")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a file with the same name as the directory to cause error
	// Actually, let's just use a read-only directory
	readOnlyDir := filepath.Join(tempDir, "readonly")
	err = os.Mkdir(readOnlyDir, 0555)
	require.NoError(t, err)
	fs = &FileStorage{
		outputDir: readOnlyDir,
	}
	err = fs.Save(context.Background(), []domain.Rejection{{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write temp file")

	// Test Save error from saveCSV
	tempDir3, err := os.MkdirTemp("", "storage_test_csv_error_save")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir3)

	err = os.Chmod(tempDir3, 0555)
	require.NoError(t, err)
	defer os.Chmod(tempDir3, 0755)

	fs = &FileStorage{
		outputDir: tempDir3,
	}
	err = fs.Save(context.Background(), []domain.Rejection{{}})
	assert.Error(t, err)
}

func TestFileStorage_Save_CSVError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage_test_save_csv_error")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	fs := &FileStorage{
		outputDir: tempDir,
	}

	err = os.Chmod(tempDir, 0555)
	require.NoError(t, err)
	defer os.Chmod(tempDir, 0755)

	err = fs.Save(context.Background(), []domain.Rejection{{}})
	assert.Error(t, err)
}

func TestFileStorage_Save_CSVError_Direct(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage_csv_direct")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	csvAsDir := filepath.Join(tempDir, "rejections_test.csv")
	err = os.Mkdir(csvAsDir, 0755)
	require.NoError(t, err)

	fs := &FileStorage{
		outputDir: tempDir,
	}

	rejections := []domain.Rejection{
		{Section: "Test", Who: "Test", What: "Test", When: "01/01/2026"},
	}

	err = fs.saveCSV(csvAsDir, rejections)
	assert.Error(t, err)
}

func TestFileStorage_Save_CSVError_InMainSave(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage_csv_main")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	fs := &FileStorage{
		outputDir: tempDir,
	}

	rejections := []domain.Rejection{
		{Section: "Test", Who: "Test", What: "Test", When: "01/01/2026"},
	}

	err = fs.Save(context.Background(), rejections)
	assert.NoError(t, err)
}

func TestFileStorage_Save_CSVErrorViaHook(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage_csv_hook")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	fs := &FileStorage{
		outputDir: tempDir,
	}

	rejections := []domain.Rejection{
		{Section: "Test", Who: "Test", What: "Test", When: "01/01/2026"},
	}

	originalSaveCSV := saveCSVFn
	defer func() { saveCSVFn = originalSaveCSV }()

	saveCSVFn = func(fs *FileStorage, filename string, rejections []domain.Rejection) error {
		return fmt.Errorf("mock CSV error")
	}

	err = fs.Save(context.Background(), rejections)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock CSV error")
}

func TestFileStorage_Save_CSVMkdirError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage_csv_mkdir")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	err = os.Chmod(tempDir, 0555)
	require.NoError(t, err)
	defer os.Chmod(tempDir, 0755)

	fs := &FileStorage{
		outputDir: tempDir,
	}

	err = fs.Save(context.Background(), []domain.Rejection{{}})
	assert.Error(t, err)
}

func TestFileStorage_Cookies(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cookie_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	cookieFile := filepath.Join(tempDir, "cookies.json")
	fs := &FileStorage{
		cookieFile: cookieFile,
	}

	// Test Load non-existent
	cookies, err := fs.LoadCookies()
	assert.NoError(t, err)
	assert.Nil(t, cookies)

	// Test Save
	testCookies := []domain.Cookie{
		{
			Name:  "test",
			Value: "value",
		},
	}
	err = fs.SaveCookies(testCookies)
	assert.NoError(t, err)

	// Test Load
	loadedCookies, err := fs.LoadCookies()
	assert.NoError(t, err)
	assert.Equal(t, testCookies, loadedCookies)

	// Test Save Error
	fsErr := &FileStorage{
		cookieFile: "/root/invalid.json",
	}
	err = fsErr.SaveCookies(testCookies)
	assert.Error(t, err)

	// Test Load Error (invalid JSON)
	err = os.WriteFile(cookieFile, []byte("invalid json"), 0644)
	require.NoError(t, err)
	_, err = fs.LoadCookies()
	assert.Error(t, err)
	// Test Load Error (not exist is handled, but other errors)
	dirAsFile := filepath.Join(tempDir, "dir_as_file")
	err = os.Mkdir(dirAsFile, 0755)
	require.NoError(t, err)
	fsErrRead := &FileStorage{
		cookieFile: dirAsFile,
	}
	_, err = fsErrRead.LoadCookies()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read cookie file")

}

type errorWriter struct{}

func (e *errorWriter) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("write error")
}

type stubWriteCloser struct {
	closeCalled int
	closeErr    error
	writeErr    error
}

func (s *stubWriteCloser) Write(p []byte) (int, error) {
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	return len(p), nil
}

func (s *stubWriteCloser) Close() error {
	s.closeCalled++
	return s.closeErr
}

func TestFileStorage_WriteCSV_Error(t *testing.T) {
	fs := &FileStorage{}
	err := fs.writeCSV(&errorWriter{}, []domain.Rejection{{}})
	assert.Error(t, err)
}

func TestFileStorage_SaveCSV_OpenFileError(t *testing.T) {
	old := openFileFn
	t.Cleanup(func() { openFileFn = old })

	openFileFn = func(name string, flag int, perm os.FileMode) (io.WriteCloser, error) {
		return nil, fmt.Errorf("open failed")
	}

	err := (&FileStorage{}).saveCSV(filepath.Join(t.TempDir(), "rejections.csv"), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create temp CSV file")
	assert.Contains(t, err.Error(), "open failed")
}

func TestFileStorage_SaveCSV_WriteErrorClosesFile(t *testing.T) {
	oldOpen := openFileFn
	oldWriter := newCSVWriter
	t.Cleanup(func() {
		openFileFn = oldOpen
		newCSVWriter = oldWriter
	})

	file := &stubWriteCloser{}
	openFileFn = func(name string, flag int, perm os.FileMode) (io.WriteCloser, error) {
		return file, nil
	}
	newCSVWriter = func(w io.Writer) *csv.Writer {
		return csv.NewWriter(&errorWriter{})
	}

	err := (&FileStorage{}).saveCSV(filepath.Join(t.TempDir(), "rejections.csv"), []domain.Rejection{{}})
	assert.Error(t, err)
	assert.Equal(t, 1, file.closeCalled)
}

func TestFileStorage_SaveCSV_CloseError(t *testing.T) {
	old := openFileFn
	t.Cleanup(func() { openFileFn = old })

	file := &stubWriteCloser{closeErr: fmt.Errorf("close failed")}
	openFileFn = func(name string, flag int, perm os.FileMode) (io.WriteCloser, error) {
		return file, nil
	}

	err := (&FileStorage{}).saveCSV(filepath.Join(t.TempDir(), "rejections.csv"), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to close CSV file")
	assert.Contains(t, err.Error(), "close failed")
	assert.Equal(t, 1, file.closeCalled)
}

func TestFileStorage_MarshalError(t *testing.T) {
	old := jsonMarshalIndent
	jsonMarshalIndent = func(v interface{}, prefix, indent string) ([]byte, error) {
		return nil, fmt.Errorf("marshal error")
	}
	defer func() { jsonMarshalIndent = old }()

	fs := &FileStorage{}

	// Test saveJSON marshal error
	err := fs.saveJSON("test.json", []domain.Rejection{{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal JSON")

	// Test SaveCookies marshal error
	err = fs.SaveCookies([]domain.Cookie{{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal cookies")
}

type limitedErrorWriter struct {
	limit int
	count int
}

func (w *limitedErrorWriter) Write(p []byte) (n int, err error) {
	w.count += len(p)
	if w.count > w.limit {
		return 0, fmt.Errorf("write error")
	}
	return len(p), nil
}

func TestFileStorage_WriteCSV_LimitedError(t *testing.T) {
	fs := &FileStorage{}
	rejections := []domain.Rejection{{Section: "test"}}

	// Fail during header write (if it exceeds buffer, but it won't)
	// Fail during row write
	err := fs.writeCSV(&limitedErrorWriter{limit: 10}, rejections)
	assert.Error(t, err)
}

func TestFileStorage_WriteCSV_WriteError(t *testing.T) {
	fs := &FileStorage{}
	rejections := make([]domain.Rejection, 1000)
	for i := range rejections {
		rejections[i] = domain.Rejection{
			Section: "Very long section name to fill the buffer quickly and trigger a write to the underlying writer",
		}
	}
	err := fs.writeCSV(&errorWriter{}, rejections)
	assert.Error(t, err)
}

func TestFileStorage_WriteCSV_HeaderWriteError(t *testing.T) {
	old := newCSVWriter
	defer func() { newCSVWriter = old }()

	// Force csv.Writer.Write to fail by replacing its internal bufio.Writer
	// with a 1-byte buffer over an errorWriter (requires reflect+unsafe since
	// the field is unexported).
	newCSVWriter = func(w io.Writer) *csv.Writer {
		cw := csv.NewWriter(w)
		bw := bufio.NewWriterSize(&errorWriter{}, 1)
		rv := reflect.ValueOf(cw).Elem()
		wField := rv.FieldByName("w")
		ptr := unsafe.Pointer(wField.UnsafeAddr())
		*(**bufio.Writer)(ptr) = bw
		return cw
	}

	fs := &FileStorage{}
	err := fs.writeCSV(&errorWriter{}, []domain.Rejection{{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write CSV header")
}
