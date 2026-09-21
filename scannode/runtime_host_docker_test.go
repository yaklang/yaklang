package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

func testRuntimeDocker(t *testing.T, handler http.HandlerFunc) *localRuntimeHostDocker {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := dockerhttp.New(dockerhttp.WithHost(server.URL), dockerhttp.WithVersion("1.44"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return &localRuntimeHostDocker{client: client}
}

func TestDockerRuntimeResources(t *testing.T) {
	input := runtimeHostContainerInput{Name: "runtime", Image: "sha256:image", Network: "bridge", Args: []string{"a", "b"}, Env: []string{"K=V"}, Labels: map[string]string{runtimeHostCleanupLabel: "key"}, CPUMillicores: 750, MemoryBytes: 64 << 20, MemorySwapBytes: 128 << 20}
	var cfg dockerhttp.ContainerCreateRequest
	d := testRuntimeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/create"):
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				t.Error(err)
			}
			if r.URL.Query().Get("name") != input.Name {
				t.Error("container name lost")
			}
			io.WriteString(w, `{"Id":"created"}`)
		case strings.HasSuffix(r.URL.Path, "/start"):
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/json"):
			json.NewEncoder(w).Encode(dockerhttp.ContainerInspect{ID: "created", Image: input.Image, Config: cfg.ContainerConfig, HostConfig: cfg.HostConfig, State: &dockerhttp.ContainerState{Running: true}})
		default:
			t.Errorf("unexpected %s", r.URL)
		}
	})
	got, err := d.CreateAndStart(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Running || got.CPUMillicores != input.CPUMillicores || got.MemoryBytes != input.MemoryBytes || got.MemorySwapBytes != input.MemorySwapBytes || got.ImageID != input.Image || !reflect.DeepEqual(got.Labels, input.Labels) {
		t.Fatalf("bad mapping: %+v", got)
	}
	if cfg.HostConfig.NetworkMode != input.Network || cfg.HostConfig.RestartPolicy.Name != "unless-stopped" || !reflect.DeepEqual(cfg.Cmd, input.Args) || !reflect.DeepEqual(cfg.Env, input.Env) {
		t.Fatalf("bad create: %+v", cfg)
	}
	input.CPUMillicores = math.MaxUint64
	if _, err := d.CreateAndStart(context.Background(), input); err == nil {
		t.Fatal("resource overflow accepted")
	}
}

func TestDockerRuntimeLookup(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			key := "value=quoted\"&?"
			d := testRuntimeDocker(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1.44/containers/json" {
					var filters map[string][]string
					if err := json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters); err != nil {
						t.Error(err)
					}
					if !reflect.DeepEqual(filters["label"], []string{runtimeHostCleanupLabel + "=" + key}) || r.URL.Query().Get("all") != "1" {
						t.Error("lookup lost exact cleanup filter")
					}
					items := make([]dockerhttp.ContainerSummary, count)
					for i := range items {
						items[i].ID = "found"
					}
					json.NewEncoder(w).Encode(items)
					return
				}
				if r.URL.Path != "/v1.44/containers/found/json" {
					t.Errorf("unexpected inspect %s", r.URL)
				}
				io.WriteString(w, `{"Id":"found"}`)
			})
			got, found, err := d.FindContainer(context.Background(), key)
			if count == 2 {
				if err == nil {
					t.Fatal("ambiguous lookup accepted")
				}
				return
			}
			if err != nil || found != (count == 1) {
				t.Fatalf("%+v %v %v", got, found, err)
			}
		})
	}
}

func TestDockerRuntimeMissingFieldsAndImageIdentity(t *testing.T) {
	d := testRuntimeDocker(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"Id":""}`) })
	if _, _, err := d.ResolveImageID(context.Background(), "empty"); err == nil {
		t.Fatal("accepted empty image identity")
	}
	if _, _, err := d.Inspect(context.Background(), "minimal"); err != nil {
		t.Fatal(err)
	}
}
