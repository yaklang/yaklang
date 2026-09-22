package thirdpartyservices

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPullServiceImagesThroughEngineHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, repo, tag string
		pull            func() error
	}{
		{"postgres", "postgres", "12.4", PullPostgresImage},
		{"rabbitmq", "rabbitmq", "3-management", PullRabbitMQImage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, streamError := range []bool{false, true} {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "POST" || r.URL.Path != "/v1.44/images/create" || r.URL.Query().Get("fromImage") != tc.repo || r.URL.Query().Get("tag") != tc.tag {
						t.Errorf("bad pull request: %s %s", r.Method, r.URL)
					}
					if streamError {
						fmt.Fprintln(w, `{"error":"pull denied"}`)
					} else {
						fmt.Fprintln(w, `{"status":"downloaded"}`)
					}
				}))
				t.Setenv("DOCKER_HOST", srv.URL)
				t.Setenv("DOCKER_API_VERSION", "1.44")
				t.Setenv("DOCKER_TLS_VERIFY", "")
				t.Setenv("DOCKER_CERT_PATH", "")
				err := tc.pull()
				srv.Close()
				if streamError {
					if err == nil || !strings.Contains(err.Error(), "pull denied") {
						t.Fatalf("stream error lost: %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
