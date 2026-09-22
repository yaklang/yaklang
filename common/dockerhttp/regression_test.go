package dockerhttp_test

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

func httpClient(t *testing.T, h http.HandlerFunc, opts ...dockerhttp.Opt) *dockerhttp.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	opts = append([]dockerhttp.Opt{dockerhttp.WithHost(srv.URL), dockerhttp.WithVersion("1.44")}, opts...)
	c, err := dockerhttp.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestEscapedObjectPaths(t *testing.T) {
	for _, ref := range []string{"registry:5000/team/image:tag", "team/image@sha256:abc", "a..b", "a?b#c%20 d"} {
		t.Run(ref, func(t *testing.T) {
			c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1.44/images/"+ref+"/json" || r.URL.RawQuery != "" {
					t.Errorf("wrong request: %s", r.URL)
				}
				io.WriteString(w, `{"Id":"sha256:ok"}`)
			})
			if _, err := c.ImageInspect(context.Background(), ref); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTLSRequestAndOptionOrder(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("API-Version", "1.44") }))
	defer srv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	cfg := srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	cfg.RootCAs = pool
	for _, reverse := range []bool{false, true} {
		opts := []dockerhttp.Opt{dockerhttp.WithTLSConfig(cfg), dockerhttp.WithTimeout(time.Second), dockerhttp.WithHost(strings.Replace(srv.URL, "https:", "tcp:", 1))}
		if reverse {
			opts[0], opts[2] = opts[2], opts[0]
		}
		c, err := dockerhttp.New(opts...)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Ping(context.Background()); err != nil {
			t.Fatal(err)
		}
		if c.HTTPClient().Timeout != time.Second {
			t.Fatal("timeout lost")
		}
		c.Close()
	}
}

func TestConcurrentVersionNegotiation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.Header().Set("API-Version", "1.40")
			return
		}
		if r.URL.Path != "/v1.40/images/json" {
			t.Errorf("path=%s", r.URL.Path)
		}
		io.WriteString(w, `[]`)
	}))
	defer srv.Close()
	c, err := dockerhttp.New(dockerhttp.WithHost(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.ImageList(context.Background(), dockerhttp.ImageListOptions{}); err != nil {
				t.Error(err)
			}
			_ = c.ClientVersion()
		}()
	}
	wg.Wait()
}

func TestCreateRollbackAfterCancelOrInspectFailure(t *testing.T) {
	for _, scenario := range []string{"cancel", "inspect", "cleanup"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			removed := make(chan struct{}, 1)
			c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1.44/containers/create":
					io.WriteString(w, `{"Id":"created"}`)
				case strings.HasSuffix(r.URL.Path, "/start"):
					if scenario == "cancel" {
						cancel()
						<-r.Context().Done()
						return
					}
					w.WriteHeader(204)
				case strings.HasSuffix(r.URL.Path, "/json"):
					http.Error(w, "broken inspect", 500)
				case r.Method == "DELETE":
					removed <- struct{}{}
					if scenario == "cleanup" {
						http.Error(w, "cleanup denied", 403)
					} else {
						w.WriteHeader(204)
					}
				default:
					t.Errorf("unexpected %s", r.URL)
				}
			})
			_, err := c.CreateAndStart(ctx, &dockerhttp.ContainerConfig{Image: "test"}, nil, "")
			if err == nil {
				t.Fatal("expected failure")
			}
			if scenario == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if scenario == "cleanup" && !strings.Contains(err.Error(), "cleanup denied") {
				t.Fatal(err)
			}
			select {
			case <-removed:
			default:
				t.Fatal("created container leaked")
			}
		})
	}
}

func TestMalformedImageStreams(t *testing.T) {
	for _, body := range []string{`not json`, `{"status":`, `{"status":"ok"}{"errorDetail":{"message":"failed"}}`} {
		t.Run(fmt.Sprintf("%q", body), func(t *testing.T) {
			c := httpClient(t, func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); io.WriteString(w, body) })
			if err := c.ImageLoad(context.Background(), strings.NewReader("archive")); err == nil {
				t.Fatal("accepted corrupt/error stream")
			}
		})
	}
}

func TestInvalidAPIVersions(t *testing.T) {
	for _, version := range []string{"1", "../images", "1.44?bad", "one.two", "1.999999"} {
		if _, err := dockerhttp.New(dockerhttp.WithVersion(version)); err == nil {
			t.Errorf("accepted %q", version)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("API-Version", "bad-version") }))
	defer srv.Close()
	c, err := dockerhttp.New(dockerhttp.WithHost(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.ImageList(context.Background(), dockerhttp.ImageListOptions{}); err == nil {
		t.Fatal("accepted invalid server version")
	}
}

func TestPublishedPortsAndMounts(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		// Check actual Engine field names rather than merely round-tripping our DTO.
		for _, part := range []string{`"ExposedPorts":{"5432/tcp":{}}`, `"HostIp":"127.0.0.1"`, `"HostPort":"5435"`, `"Source":"/host/data"`, `"Target":"/var/lib/postgresql/data"`, `"Type":"bind"`} {
			if !strings.Contains(string(body), part) {
				t.Errorf("missing %s in %s", part, body)
			}
		}
		io.WriteString(w, `{"Id":"service"}`)
	})
	_, err := c.ContainerCreate(context.Background(), &dockerhttp.ContainerConfig{Image: "postgres", ExposedPorts: map[string]struct{}{"5432/tcp": {}}}, &dockerhttp.HostConfig{PortBindings: dockerhttp.PortMap{"5432/tcp": {{HostIP: "127.0.0.1", HostPort: "5435"}}}, Mounts: []dockerhttp.Mount{{Type: "bind", Source: "/host/data", Target: "/var/lib/postgresql/data"}}}, "postgres")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAlreadyStartedIsSuccess(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotModified) })
	if err := c.ContainerStart(context.Background(), "running"); err != nil {
		t.Fatal(err)
	}
}
