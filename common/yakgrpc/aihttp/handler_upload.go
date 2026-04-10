package aihttp

import (
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const uploadFormMemoryLimit = 32 << 20

func (gw *AIAgentHTTPGateway) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(uploadFormMemoryLimit); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}

	src, fileHeader, err := readUploadFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer src.Close()

	savedName := buildSavedFilename(fileHeader.Filename)
	savedPath := filepath.Join(gw.uploadDir, savedName)

	dst, err := os.OpenFile(savedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create upload file failed: "+err.Error())
		return
	}
	defer dst.Close()

	size, err := io.Copy(dst, src)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save upload file failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, UploadFileResponse{
		Path:         savedPath,
		Filename:     savedName,
		OriginalName: fileHeader.Filename,
		Size:         size,
		ContentType:  fileHeader.Header.Get("Content-Type"),
	})
}

func readUploadFile(r *http.Request) (multipart.File, *multipart.FileHeader, error) {
	if file, fileHeader, err := r.FormFile("file"); err == nil {
		return file, fileHeader, nil
	}

	if r.MultipartForm == nil {
		return nil, nil, http.ErrMissingFile
	}
	for _, fileHeaders := range r.MultipartForm.File {
		if len(fileHeaders) == 0 {
			continue
		}
		file, err := fileHeaders[0].Open()
		if err != nil {
			return nil, nil, err
		}
		return file, fileHeaders[0], nil
	}
	return nil, nil, http.ErrMissingFile
}

func buildSavedFilename(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return uuid.NewString()
	}

	ext := filepath.Ext(base)
	prefix := strings.TrimSuffix(base, ext)
	if prefix == "" {
		return uuid.NewString() + ext
	}
	return prefix + "_" + uuid.NewString() + ext
}
