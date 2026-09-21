package dockerhttp_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

// mockEngine is a minimal Docker Engine HTTP API for contract tests.
type mockEngine struct {
	mu sync.Mutex

	APIVersion string // header value for /_ping

	// behavior knobs
	PingStatus      int
	PingDelay       time.Duration
	RejectNegotiate bool // return empty API-Version

	Images    map[string]dockerhttp.ImageInspect // key = name or id
	ImageList []dockerhttp.ImageSummary

	Containers map[string]*dockerhttp.ContainerInspect
	// createBody captures last create JSON for fidelity assertions
	LastCreateBody []byte
	LastCreateName string
	CreateFail     bool
	StartFailIDs   map[string]bool
	RemovedIDs     []string
	StopCalls      []string
	LoadHandler    func(w http.ResponseWriter, r *http.Request)
	LoadCalls      int

	// filter capture
	LastFiltersQuery string
}

func newMockEngine() *mockEngine {
	return &mockEngine{
		APIVersion:   "1.44",
		PingStatus:   http.StatusOK,
		Images:       map[string]dockerhttp.ImageInspect{},
		Containers:   map[string]*dockerhttp.ContainerInspect{},
		StartFailIDs: map[string]bool{},
	}
}

func (m *mockEngine) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", m.handlePing)
	mux.HandleFunc("/v1.44/_ping", m.handlePing)
	mux.HandleFunc("/v1.24/_ping", m.handlePing)
	mux.HandleFunc("/version", m.handleVersion)
	mux.HandleFunc("/v1.44/version", m.handleVersion)
	mux.HandleFunc("/v1.24/version", m.handleVersion)

	// Catch-all for versioned paths
	mux.HandleFunc("/", m.handleAll)
	return mux
}

func (m *mockEngine) handleAll(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	// strip /v1.XX prefix
	if strings.HasPrefix(p, "/v") {
		if i := strings.Index(p[1:], "/"); i >= 0 {
			p = p[1+i:]
		}
	}
	switch {
	case p == "/images/json" && r.Method == http.MethodGet:
		m.handleImageList(w, r)
	case strings.HasPrefix(p, "/images/") && strings.HasSuffix(p, "/json") && r.Method == http.MethodGet:
		m.handleImageInspect(w, r, p)
	case p == "/images/load" && r.Method == http.MethodPost:
		m.handleImageLoad(w, r)
	case strings.HasPrefix(p, "/images/") && strings.HasSuffix(p, "/tag") && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusCreated)
	case strings.HasPrefix(p, "/images/") && r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	case p == "/images/get" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/x-tar")
		_, _ = w.Write([]byte("TARDATA"))
	case p == "/containers/json" && r.Method == http.MethodGet:
		m.handleContainerList(w, r)
	case p == "/containers/create" && r.Method == http.MethodPost:
		m.handleContainerCreate(w, r)
	case strings.HasPrefix(p, "/containers/") && strings.HasSuffix(p, "/json") && r.Method == http.MethodGet:
		m.handleContainerInspect(w, r, p)
	case strings.HasPrefix(p, "/containers/") && strings.HasSuffix(p, "/start") && r.Method == http.MethodPost:
		m.handleContainerStart(w, r, p)
	case strings.HasPrefix(p, "/containers/") && strings.HasSuffix(p, "/stop") && r.Method == http.MethodPost:
		m.handleContainerStop(w, r, p)
	case strings.HasPrefix(p, "/containers/") && strings.HasSuffix(p, "/restart") && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(p, "/containers/") && strings.HasSuffix(p, "/kill") && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(p, "/containers/") && strings.HasSuffix(p, "/export") && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/x-tar")
		_, _ = w.Write([]byte("CTAR"))
	case strings.HasPrefix(p, "/containers/") && strings.HasSuffix(p, "/logs") && r.Method == http.MethodGet:
		_, _ = w.Write([]byte("logline\n"))
	case strings.HasPrefix(p, "/containers/") && r.Method == http.MethodDelete:
		m.handleContainerRemove(w, r, p)
	default:
		http.NotFound(w, r)
	}
}

func (m *mockEngine) handlePing(w http.ResponseWriter, r *http.Request) {
	if m.PingDelay > 0 {
		select {
		case <-time.After(m.PingDelay):
		case <-r.Context().Done():
			return
		}
	}
	status := m.PingStatus
	if status == 0 {
		status = http.StatusOK
	}
	if !m.RejectNegotiate {
		w.Header().Set("API-Version", m.APIVersion)
	}
	w.Header().Set("OSType", "linux")
	w.Header().Set("Docker-Experimental", "false")
	w.WriteHeader(status)
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte("OK"))
	}
}

func (m *mockEngine) handleVersion(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(dockerhttp.VersionInfo{
		Version:    "25.0.0",
		APIVersion: m.APIVersion,
		Os:         "linux",
		Arch:       "amd64",
	})
}

func (m *mockEngine) handleImageList(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.ImageList
	if list == nil {
		list = []dockerhttp.ImageSummary{}
	}
	_ = json.NewEncoder(w).Encode(list)
}

func (m *mockEngine) handleImageInspect(w http.ResponseWriter, r *http.Request, p string) {
	// /images/{name}/json
	name := strings.TrimSuffix(strings.TrimPrefix(p, "/images/"), "/json")
	name, _ = urlPathUnescape(name)
	m.mu.Lock()
	img, ok := m.Images[name]
	m.mu.Unlock()
	if !ok {
		writeErr(w, http.StatusNotFound, "No such image: "+name)
		return
	}
	_ = json.NewEncoder(w).Encode(img)
}

func (m *mockEngine) handleImageLoad(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.LoadCalls++
	h := m.LoadHandler
	m.mu.Unlock()
	if h != nil {
		h(w, r)
		return
	}
	_, _ = io.Copy(io.Discard, r.Body)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"Loaded image: test:latest"}` + "\n"))
}

func (m *mockEngine) handleContainerList(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.LastFiltersQuery = r.URL.Query().Get("filters")
	defer m.mu.Unlock()

	var out []dockerhttp.ContainerSummary
	for _, c := range m.Containers {
		s := dockerhttp.ContainerSummary{
			ID:     c.ID,
			Names:  []string{c.Name},
			Image:  "",
			State:  "",
			Status: "",
		}
		if c.Config != nil {
			s.Image = c.Config.Image
			s.Labels = c.Config.Labels
		}
		if c.State != nil {
			s.State = c.State.Status
			s.Status = c.State.Status
		}
		// apply label filters if present
		if fq := m.LastFiltersQuery; fq != "" {
			var f map[string][]string
			if json.Unmarshal([]byte(fq), &f) == nil {
				if labels, ok := f["label"]; ok {
					match := true
					for _, lv := range labels {
						if !labelMatch(s.Labels, lv) {
							match = false
							break
						}
					}
					if !match {
						continue
					}
				}
			}
		}
		out = append(out, s)
	}
	if out == nil {
		out = []dockerhttp.ContainerSummary{}
	}
	_ = json.NewEncoder(w).Encode(out)
}

func labelMatch(labels map[string]string, spec string) bool {
	if labels == nil {
		return false
	}
	if i := strings.IndexByte(spec, '='); i >= 0 {
		return labels[spec[:i]] == spec[i+1:]
	}
	_, ok := labels[spec]
	return ok
}

func (m *mockEngine) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	m.LastCreateBody = body
	m.LastCreateName = r.URL.Query().Get("name")
	if m.CreateFail {
		writeErr(w, http.StatusInternalServerError, "create failed")
		return
	}
	id := "cid-" + m.LastCreateName
	if id == "cid-" {
		id = "cid-anon"
	}
	var req dockerhttp.ContainerCreateRequest
	_ = json.Unmarshal(body, &req)
	ins := &dockerhttp.ContainerInspect{
		ID:         id,
		Name:       "/" + m.LastCreateName,
		Image:      "sha256:img",
		Config:     req.ContainerConfig,
		HostConfig: req.HostConfig,
		State:      &dockerhttp.ContainerState{Status: "created", Running: false},
		Created:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if ins.Config == nil {
		ins.Config = &dockerhttp.ContainerConfig{}
	}
	m.Containers[id] = ins
	_ = json.NewEncoder(w).Encode(dockerhttp.ContainerCreateResponse{ID: id})
}

func (m *mockEngine) handleContainerInspect(w http.ResponseWriter, r *http.Request, p string) {
	id := strings.TrimSuffix(strings.TrimPrefix(p, "/containers/"), "/json")
	id, _ = urlPathUnescape(id)
	m.mu.Lock()
	c, ok := m.Containers[id]
	m.mu.Unlock()
	if !ok {
		writeErr(w, http.StatusNotFound, "No such container: "+id)
		return
	}
	_ = json.NewEncoder(w).Encode(c)
}

func (m *mockEngine) handleContainerStart(w http.ResponseWriter, r *http.Request, p string) {
	id := strings.TrimSuffix(strings.TrimPrefix(p, "/containers/"), "/start")
	id, _ = urlPathUnescape(id)
	m.mu.Lock()
	fail := m.StartFailIDs[id]
	c, ok := m.Containers[id]
	m.mu.Unlock()
	if !ok {
		writeErr(w, http.StatusNotFound, "No such container")
		return
	}
	if fail {
		writeErr(w, http.StatusInternalServerError, "start failed")
		return
	}
	c.State = &dockerhttp.ContainerState{Status: "running", Running: true}
	w.WriteHeader(http.StatusNoContent)
}

func (m *mockEngine) handleContainerStop(w http.ResponseWriter, r *http.Request, p string) {
	id := strings.TrimSuffix(strings.TrimPrefix(p, "/containers/"), "/stop")
	id, _ = urlPathUnescape(id)
	m.mu.Lock()
	m.StopCalls = append(m.StopCalls, id)
	c, ok := m.Containers[id]
	m.mu.Unlock()
	if !ok {
		writeErr(w, http.StatusNotFound, "No such container")
		return
	}
	if c.State != nil && !c.State.Running {
		w.WriteHeader(http.StatusNotModified) // already stopped
		return
	}
	if c.State != nil {
		c.State.Running = false
		c.State.Status = "exited"
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *mockEngine) handleContainerRemove(w http.ResponseWriter, r *http.Request, p string) {
	id := strings.TrimPrefix(p, "/containers/")
	id, _ = urlPathUnescape(id)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.Containers[id]; !ok {
		writeErr(w, http.StatusNotFound, "No such container")
		return
	}
	delete(m.Containers, id)
	m.RemovedIDs = append(m.RemovedIDs, id)
	w.WriteHeader(http.StatusNoContent)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
}

func urlPathUnescape(s string) (string, error) {
	return s, nil // paths kept with slashes
}

// startUnixMock serves the mock over a temp unix socket and returns a client + cleanup.
func startUnixMock(t *testing.T, m *mockEngine) *dockerhttp.Client {
	t.Helper()
	dir, err := os.MkdirTemp("", "dh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "docker.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	srv := &http.Server{Handler: m.Handler()}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = ln.Close()
		_ = os.Remove(sock)
	})
	c, err := dockerhttp.New(
		dockerhttp.WithHost("unix://"+sock),
		dockerhttp.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
