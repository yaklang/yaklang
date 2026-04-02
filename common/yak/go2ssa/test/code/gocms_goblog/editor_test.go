package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestEditorHandleFileAttachments(t *testing.T) {
	a := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	_ = a.initConfig(false)

	// Helper function to create a multipart form request
	createMultipartRequest := func(files map[string]map[string]string) *http.Request {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		for fieldName, files := range files {
			for fileName, fileContent := range files {
				part, err := writer.CreateFormFile(fieldName, fileName)
				if err != nil {
					t.Fatalf("Failed to create form file: %v", err)
				}
				_, err = io.Copy(part, strings.NewReader(fileContent))
				if err != nil {
					t.Fatalf("Failed to copy file content: %v", err)
				}
			}
		}
		writer.Close()
		req := httptest.NewRequest(http.MethodPost, "/", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		return req
	}

	t.Run("Valid GPX file", func(t *testing.T) {
		req := createMultipartRequest(map[string]map[string]string{
			"files1": {
				"track.gpx": "<gpx>valid gpx content</gpx>",
			},
		})
		images, gpx, statusCode, err := a.editorHandleFileAttachments(req)
		if err != nil || statusCode != 0 {
			t.Fatalf("Expected no error, got %v with status code %d", err, statusCode)
		}
		if len(images) != 0 {
			t.Fatalf("Expected no images, got %d", len(images))
		}
		if gpx == "" {
			t.Fatal("Expected GPX content, got empty string")
		}
	})

	t.Run("Invalid GPX file", func(t *testing.T) {
		req := createMultipartRequest(map[string]map[string]string{
			"files1": {
				"track.gpx": "invalid gpx content",
			},
		})
		_, _, statusCode, err := a.editorHandleFileAttachments(req)
		if err == nil || statusCode != http.StatusBadRequest {
			t.Fatalf("Expected error with status code %d, got %v with status code %d", http.StatusBadRequest, err, statusCode)
		}
	})

	t.Run("Multiple files including GPX", func(t *testing.T) {
		req := createMultipartRequest(map[string]map[string]string{
			"files1": {
				"image1.jpg": "image content",
				"image2.png": "image content",
				"track.gpx":  "<gpx>valid gpx content</gpx>",
			},
			"files2": {
				"image3.jpg": "image content",
			},
		})
		images, gpx, statusCode, err := a.editorHandleFileAttachments(req)
		if err != nil || statusCode != 0 {
			t.Fatalf("Expected no error, got %v with status code %d", err, statusCode)
		}
		if len(images) != 3 {
			t.Fatalf("Expected 3 image, got %d", len(images))
		}
		if gpx == "" {
			t.Fatal("Expected GPX content, got empty string")
		}
	})

	t.Run("No files", func(t *testing.T) {
		req := createMultipartRequest(map[string]map[string]string{})
		images, gpx, statusCode, err := a.editorHandleFileAttachments(req)
		if err != nil || statusCode != 0 {
			t.Fatalf("Expected no error, got %v with status code %d", err, statusCode)
		}
		if len(images) != 0 {
			t.Fatalf("Expected no images, got %d", len(images))
		}
		if gpx != "" {
			t.Fatalf("Expected no GPX content, got %s", gpx)
		}
	})
}

func TestServeEditorPost(t *testing.T) {
	a := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	_ = a.initConfig(false)

	t.Run("Unknown action", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Form = url.Values{"editoraction": {"abc"}}
		rec := httptest.NewRecorder()

		a.serveEditorPost(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("Expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Unknown or missing editoraction") {
			t.Fatalf("Expected error message, got %s", rec.Body.String())
		}
	})

	t.Run("Help GPX with valid file", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("files", "track1.gpx")
		_, _ = part.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="ExampleCreator">
  <trk>
	<name>Example Track</name>
	<trkseg>
	  <trkpt lat="48.208174" lon="16.373819">
		<ele>160</ele>
		<time>2023-03-01T12:00:00Z</time>
	  </trkpt>
	  <trkpt lat="48.208255" lon="16.374123">
		<ele>162</ele>
		<time>2023-03-01T12:01:00Z</time>
	  </trkpt>
	</trkseg>
  </trk>
</gpx>`))
		part, _ = writer.CreateFormFile("files", "track2.gpx")
		_, _ = part.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="AnotherExampleCreator">
  <trk>
	<name>Another Example Track</name>
	<trkseg>
	  <trkpt lat="40.712776" lon="-74.005974">
		<ele>10</ele>
		<time>2023-03-02T08:00:00Z</time>
	  </trkpt>
	  <trkpt lat="40.713776" lon="-74.006974">
		<ele>12</ele>
		<time>2023-03-02T08:01:00Z</time>
	  </trkpt>
	</trkseg>
  </trk>
</gpx>`))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Form = url.Values{"editoraction": {"helpgpx"}}
		rec := httptest.NewRecorder()

		a.serveEditorPost(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "gpx") {
			t.Fatalf("Expected GPX response, got %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "48.208174") {
			t.Fatalf("Expected to include first file points, got %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "40.712776") {
			t.Fatalf("Expected to include second file points, got %s", rec.Body.String())
		}
	})
}
