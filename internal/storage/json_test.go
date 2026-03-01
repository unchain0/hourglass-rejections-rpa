package storage

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hourglass-rejections-rpa/internal/config"
	"hourglass-rejections-rpa/internal/domain"

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

	rejeicoes := []domain.Rejeicao{
		{
			Secao:     "Secao 1",
			Quem:      "Quem 1",
			OQue:      "O Que 1",
			PraQuando: "Pra Quando 1",
			Timestamp: time.Now().Truncate(time.Second),
		},
	}

	err = fs.Save(context.Background(), rejeicoes)
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
	var savedRejeicoes []domain.Rejeicao
	err = json.Unmarshal(jsonData, &savedRejeicoes)
	assert.NoError(t, err)
	assert.Equal(t, rejeicoes, savedRejeicoes)

	// Verify CSV content
	csvData, err := os.Open(csvFile)
	assert.NoError(t, err)
	defer csvData.Close()
	reader := csv.NewReader(csvData)
	records, err := reader.ReadAll()
	assert.NoError(t, err)
	assert.Len(t, records, 2) // Header + 1 row
	assert.Equal(t, []string{"secao", "quem", "oque", "pra_quando", "timestamp"}, records[0])
	assert.Equal(t, rejeicoes[0].Secao, records[1][0])
	assert.Equal(t, rejeicoes[0].Quem, records[1][1])
	assert.Equal(t, rejeicoes[0].OQue, records[1][2])
	assert.Equal(t, rejeicoes[0].PraQuando, records[1][3])
	assert.Equal(t, rejeicoes[0].Timestamp.Format(time.RFC3339), records[1][4])
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
	err = fs.Save(context.Background(), []domain.Rejeicao{{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write JSON file")

	// Test Save error from saveCSV
	tempDir3, err := os.MkdirTemp("", "storage_test_csv_error_save")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir3)

	// We need to know the timestamp to pre-create the CSV directory
	timestamp := time.Now().Format("20060102_1504")
	csvDir := filepath.Join(tempDir3, fmt.Sprintf("rejeicoes_%s.csv", timestamp))
	err = os.MkdirAll(csvDir, 0755)
	require.NoError(t, err)

	fs = &FileStorage{
		outputDir: tempDir3,
	}
	err = fs.Save(context.Background(), []domain.Rejeicao{{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create CSV file")
}



func TestFileStorage_Save_CSVError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage_test_save_csv_error")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	fs := &FileStorage{
		outputDir: tempDir,
	}

	// Pre-create a directory with the name that Save will try to use for the CSV file
	timestamp := time.Now().Format("20060102_1504")
	csvDir := filepath.Join(tempDir, fmt.Sprintf("rejeicoes_%s.csv", timestamp))
	err = os.MkdirAll(csvDir, 0755)
	require.NoError(t, err)

	err = fs.Save(context.Background(), []domain.Rejeicao{{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create CSV file")
}


func TestFileStorage_SaveCSV_Error(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage_csv_error")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	fs := &FileStorage{
		outputDir: tempDir,
	}

	// Test os.Create error
	err = fs.saveCSV(tempDir, []domain.Rejeicao{{}}) // tempDir is a directory, os.Create(tempDir) should fail
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create CSV file")
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

func TestFileStorage_WriteCSV_Error(t *testing.T) {
	fs := &FileStorage{}
	err := fs.writeCSV(&errorWriter{}, []domain.Rejeicao{{}})
	assert.Error(t, err)
}


func TestFileStorage_MarshalError(t *testing.T) {
	old := jsonMarshalIndent
	jsonMarshalIndent = func(v interface{}, prefix, indent string) ([]byte, error) {
		return nil, fmt.Errorf("marshal error")
	}
	defer func() { jsonMarshalIndent = old }()

	fs := &FileStorage{}
	
	// Test saveJSON marshal error
	err := fs.saveJSON("test.json", []domain.Rejeicao{{}})
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
	rejeicoes := []domain.Rejeicao{{Secao: "test"}}
	
	// Fail during header write (if it exceeds buffer, but it won't)
	// Fail during row write
	err := fs.writeCSV(&limitedErrorWriter{limit: 10}, rejeicoes)
	assert.Error(t, err)
}

func TestFileStorage_WriteCSV_WriteError(t *testing.T) {
	fs := &FileStorage{}
	rejeicoes := make([]domain.Rejeicao, 1000)
	for i := range rejeicoes {
		rejeicoes[i] = domain.Rejeicao{
			Secao: "Very long section name to fill the buffer quickly and trigger a write to the underlying writer",
		}
	}
	err := fs.writeCSV(&errorWriter{}, rejeicoes)
	assert.Error(t, err)
}
