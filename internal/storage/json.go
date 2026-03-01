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

	"hourglass-rejections-rpa/internal/config"
	"hourglass-rejections-rpa/internal/domain"
)

var (
	jsonMarshalIndent = json.MarshalIndent
)

type FileStorage struct {

	outputDir  string
	cookieFile string
}

func New(cfg *config.Config) *FileStorage {
	return &FileStorage{
		outputDir:  cfg.OutputDir,
		cookieFile: cfg.CookieFile,
	}
}

func (fs *FileStorage) Save(ctx context.Context, rejeicoes []domain.Rejeicao) error {
	if err := os.MkdirAll(fs.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_1504")
	jsonFilename := filepath.Join(fs.outputDir, fmt.Sprintf("rejeicoes_%s.json", timestamp))
	csvFilename := filepath.Join(fs.outputDir, fmt.Sprintf("rejeicoes_%s.csv", timestamp))

	if err := fs.saveJSON(jsonFilename, rejeicoes); err != nil {
		return err
	}

	if err := fs.saveCSV(csvFilename, rejeicoes); err != nil {
		return err
	}

	return nil
}

func (fs *FileStorage) saveJSON(filename string, rejeicoes []domain.Rejeicao) error {
	data, err := jsonMarshalIndent(rejeicoes, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}


	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	return nil
}

func (fs *FileStorage) saveCSV(filename string, rejeicoes []domain.Rejeicao) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	return fs.writeCSV(file, rejeicoes)
}

func (fs *FileStorage) writeCSV(w io.Writer, rejeicoes []domain.Rejeicao) error {
	writer := csv.NewWriter(w)

	// Header
	header := []string{"secao", "quem", "oque", "pra_quando", "timestamp"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Data
	for _, r := range rejeicoes {
		row := []string{
			r.Secao,
			r.Quem,
			r.OQue,
			r.PraQuando,
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

func (fs *FileStorage) SaveCookies(cookies []domain.Cookie) error {
	data, err := jsonMarshalIndent(cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cookies: %w", err)
	}


	if err := os.WriteFile(fs.cookieFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write cookie file: %w", err)
	}

	return nil
}
