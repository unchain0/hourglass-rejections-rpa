package storage

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"hourglass-rejections-rpa/src/domain_models"
	"hourglass-rejections-rpa/src/integrations/config"
)

var (
	jsonMarshalIndent = json.MarshalIndent
	newCSVWriter      = func(w io.Writer) *csv.Writer { return csv.NewWriter(w) }
	saveCSVFn         = (*FileStorage).saveCSV
	openFileFn        = func(name string, flag int, perm os.FileMode) (io.WriteCloser, error) {
		return os.OpenFile(name, flag, perm)
	}
)

// FileStorage persists rejection outputs and authentication cookies on disk.
type FileStorage struct {
	outputDir  string
	cookieFile string
}

// New creates a FileStorage using paths from Config.
func New(cfg *config.Config) *FileStorage {
	return &FileStorage{
		outputDir:  cfg.OutputDir,
		cookieFile: cfg.CookieFile,
	}
}

// Save persists rejections to JSON and CSV files.
func (fs *FileStorage) Save(ctx context.Context, rejections []domain.Rejection) error {
	if err := os.MkdirAll(fs.outputDir, 0750); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	now := time.Now()
	timestamp := now.Format("20060102_150405") + fmt.Sprintf("_%09d", now.Nanosecond())
	jsonFilename := filepath.Join(fs.outputDir, fmt.Sprintf("rejections_%s.json", timestamp))
	csvFilename := filepath.Join(fs.outputDir, fmt.Sprintf("rejections_%s.csv", timestamp))

	if err := fs.saveJSON(jsonFilename, rejections); err != nil {
		return err
	}

	if err := saveCSVFn(fs, csvFilename, rejections); err != nil {
		return err
	}

	return nil
}

func (fs *FileStorage) saveJSON(filename string, rejections []domain.Rejection) error {
	data, err := jsonMarshalIndent(rejections, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return writeFileAtomic(filename, data, 0600)
}

func writeFileAtomic(filename string, data []byte, perm os.FileMode) error {
	tmp := filename + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	return os.Rename(tmp, filename)
}

func (fs *FileStorage) saveCSV(filename string, rejections []domain.Rejection) error {
	tmp := filename + ".tmp"
	file, err := openFileFn(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create temp CSV file: %w", err)
	}
	defer func() { _ = os.Remove(tmp) }()

	if err := fs.writeCSV(file, rejections); err != nil {
		_ = file.Close()
		return err
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close CSV file: %w", err)
	}

	return os.Rename(tmp, filename)
}

func (fs *FileStorage) writeCSV(w io.Writer, rejections []domain.Rejection) error {
	writer := newCSVWriter(w)

	header := []string{"section", "who", "what", "when", "timestamp"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	for _, r := range rejections {
		row := []string{
			r.Section,
			r.Who,
			r.What,
			r.When,
			r.Timestamp.Format(time.RFC3339),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	return nil
}

// LoadCookies reads cookies from the configured cookie file.
func (fs *FileStorage) LoadCookies() ([]domain.Cookie, error) {
	data, err := os.ReadFile(fs.cookieFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read cookie file: %w", err)
	}

	var cookies []domain.Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cookies: %w", err)
	}

	return cookies, nil
}

// SaveCookies writes cookies to the configured cookie file.
func (fs *FileStorage) SaveCookies(cookies []domain.Cookie) error {
	data, err := jsonMarshalIndent(cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cookies: %w", err)
	}

	if err := os.WriteFile(fs.cookieFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write cookie file: %w", err)
	}

	return nil
}
