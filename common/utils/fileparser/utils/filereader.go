package utils

import (
	"archive/zip"
	"io"
	"path/filepath"
)

type FileReader struct {
	CurrentDir string
	zipFile    *zip.ReadCloser
}

func NewFileReader(filePath string) (*FileReader, error) {
	zipFile, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	return &FileReader{zipFile: zipFile}, nil
}

func (r *FileReader) Close() error {
	return r.zipFile.Close()
}

func (r *FileReader) Cd(filePath string) error {
	r.CurrentDir = filepath.Join(r.CurrentDir, filePath)
	return nil
}
func (r *FileReader) SetCurrentFilePath(filePath string) error {
	r.CurrentDir = filepath.Dir(filePath)
	return nil
}

func (r *FileReader) ReadFile(filePath string) ([]byte, error) {
	if filePath == "" {
		filePath = r.CurrentDir
	}
	file, err := r.zipFile.Open(filepath.Join(r.CurrentDir, filePath))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func (r *FileReader) ListFiles() ([]string, error) {
	files := make([]string, 0)
	for _, file := range r.zipFile.File {
		files = append(files, file.Name)
	}
	return files, nil
}
