package crep

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompressedMITMStaticAssets(t *testing.T) {
	err := filepath.WalkDir("static", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.HasSuffix(name, ".embed.gz") {
			return nil
		}
		name = filepath.ToSlash(name)
		want, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		got, err := staticFS.ReadFile(name)
		if err != nil {
			return err
		}
		if !bytes.Equal(want, got) {
			t.Errorf("static resource differs: %s; run go generate ./common/crep", name)
		}
		rsp := &http.Response{Request: &http.Request{URL: &url.URL{Path: "/" + name}}, Header: make(http.Header)}
		if err := handleBuildInMITMDefaultPageResponse(rsp); err != nil {
			return err
		}
		body, err := io.ReadAll(rsp.Body)
		rsp.Body.Close()
		if err != nil {
			return err
		}
		if !bytes.Equal(want, body) || rsp.ContentLength != int64(len(want)) || rsp.StatusCode != http.StatusOK {
			t.Errorf("static HTTP response differs: %s", name)
		}
		if strings.HasSuffix(name, ".css") && rsp.Header.Get("Content-Type") != "text/css" {
			t.Fatal(rsp.Header)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rsp := &http.Response{Request: &http.Request{URL: &url.URL{Path: "/"}}, Header: make(http.Header)}
	if err := handleBuildInMITMDefaultPageResponse(rsp); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("static/navtab.html")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rsp.Body)
	rsp.Body.Close()
	if err != nil || !bytes.Equal(want, got) || rsp.ContentLength != int64(len(want)) {
		t.Fatalf("MITM index mismatch: %v", err)
	}
}
