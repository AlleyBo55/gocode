package kiro

import (
	"fmt"
	"os"
	"path/filepath"
)

// SpecFile represents a single spec file.
type SpecFile struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// SpecFeature represents a feature spec directory.
type SpecFeature struct {
	Name  string     `json:"name"`
	Path  string     `json:"path"`
	Files []SpecFile `json:"files"`
}

// LoadSpecs reads all spec feature directories from the given base directory.
// Returns an empty slice if the directory does not exist.
func LoadSpecs(baseDir string) ([]SpecFeature, error) {
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return []SpecFeature{}, nil
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, fmt.Errorf("reading specs directory: %w", err)
	}

	var features []SpecFeature
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		featurePath := filepath.Join(baseDir, entry.Name())
		files, err := loadSpecFiles(featurePath)
		if err != nil {
			return nil, err
		}
		features = append(features, SpecFeature{
			Name:  entry.Name(),
			Path:  featurePath,
			Files: files,
		})
	}
	return features, nil
}

func loadSpecFiles(dir string) ([]SpecFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading spec feature dir: %w", err)
	}
	var files []SpecFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading spec file %s: %w", path, err)
		}
		files = append(files, SpecFile{
			Name:    entry.Name(),
			Path:    path,
			Content: string(data),
		})
	}
	return files, nil
}
