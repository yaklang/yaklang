package dockerhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

func TestImagePullConsumesStreamAndErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("API-Version", "1.44")
		w.WriteHeader(http.StatusOK)
	})
	var gotPath, gotQuery string
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/images/create") && r.Method == http.MethodPost {
			gotPath = r.URL.Path
			gotQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"status\":\"Pulling\"}\n{\"status\":\"Download complete\"}\n"))
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := dockerhttp.New(
		dockerhttp.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")),
		dockerhttp.WithHTTPClient(srv.Client()),
		dockerhttp.WithVersion("1.44"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.ImagePull(context.Background(), "alpine:3.20", dockerhttp.ImagePullOptions{}); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}
	if !strings.Contains(gotPath, "/images/create") {
		t.Fatalf("path=%s", gotPath)
	}
	if !strings.Contains(gotQuery, "fromImage=alpine") || !strings.Contains(gotQuery, "tag=3.20") {
		t.Fatalf("query=%s", gotQuery)
	}
}

func TestImagePullStreamError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("API-Version", "1.44")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/images/create") {
			msg := dockerhttp.JSONMessage{ErrorMsg: "pull access denied"}
			b, _ := json.Marshal(msg)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(append(b, '\n'))
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := dockerhttp.New(
		dockerhttp.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")),
		dockerhttp.WithHTTPClient(srv.Client()),
		dockerhttp.WithVersion("1.44"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	err = c.ImagePull(context.Background(), "private/nope:latest", dockerhttp.ImagePullOptions{})
	if err == nil {
		t.Fatal("expected stream error")
	}
	if !strings.Contains(err.Error(), "pull access denied") {
		t.Fatalf("err=%v", err)
	}
}
