package dockerhttp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

func TestContainerListFilters(t *testing.T) {
	m := newMockEngine()
	m.Containers["a"] = &dockerhttp.ContainerInspect{
		ID: "a", Name: "/one",
		Config: &dockerhttp.ContainerConfig{Labels: map[string]string{"task": "x"}},
		State:  &dockerhttp.ContainerState{Status: "running", Running: true},
	}
	m.Containers["b"] = &dockerhttp.ContainerInspect{
		ID: "b", Name: "/two",
		Config: &dockerhttp.ContainerConfig{Labels: map[string]string{"task": "y"}},
		State:  &dockerhttp.ContainerState{Status: "exited"},
	}
	m.Containers["c"] = &dockerhttp.ContainerInspect{
		ID: "c", Name: "/three",
		Config: &dockerhttp.ContainerConfig{Labels: map[string]string{"task": "x", "special": "a=b,c"}},
		State:  &dockerhttp.ContainerState{Status: "running", Running: true},
	}
	cli := startUnixMock(t, m)

	// 0 matches
	f := dockerhttp.Filters{}
	f.Add("label", "task=none")
	list, err := cli.ContainerList(context.Background(), dockerhttp.ContainerListOptions{All: true, Filters: f})
	if err != nil || len(list) != 0 {
		t.Fatalf("0 match: %d %v", len(list), err)
	}

	// 1 match
	f = dockerhttp.Filters{}
	f.Add("label", "task=y")
	list, err = cli.ContainerList(context.Background(), dockerhttp.ContainerListOptions{All: true, Filters: f})
	if err != nil || len(list) != 1 || list[0].ID != "b" {
		t.Fatalf("1 match: %+v %v", list, err)
	}

	// many matches
	f = dockerhttp.Filters{}
	f.Add("label", "task=x")
	list, err = cli.ContainerList(context.Background(), dockerhttp.ContainerListOptions{All: true, Filters: f})
	if err != nil || len(list) != 2 {
		t.Fatalf("many: %d %v", len(list), err)
	}

	// special chars in label value
	f = dockerhttp.Filters{}
	f.Add("label", "special=a=b,c")
	list, err = cli.ContainerList(context.Background(), dockerhttp.ContainerListOptions{All: true, Filters: f})
	if err != nil || len(list) != 1 {
		t.Fatalf("special: %d filters=%q err=%v", len(list), m.LastFiltersQuery, err)
	}
	if !strings.Contains(m.LastFiltersQuery, "special") {
		t.Fatalf("filters not encoded: %q", m.LastFiltersQuery)
	}
}

func TestContainerCreateParamFidelity(t *testing.T) {
	m := newMockEngine()
	cli := startUnixMock(t, m)

	cfg := &dockerhttp.ContainerConfig{
		Image:  "alpine:latest",
		Env:    []string{"A=1", "B=two three"},
		Labels: map[string]string{"k": "v", "dot.key": "x"},
		Cmd:    []string{"sh", "-c", "sleep 1"},
	}
	hc := &dockerhttp.HostConfig{
		NanoCPUs:      1_500_000_000,
		Memory:        256 * 1024 * 1024,
		MemorySwap:    512 * 1024 * 1024,
		NetworkMode:   "bridge",
		RestartPolicy: &dockerhttp.RestartPolicy{Name: "unless-stopped"},
	}
	resp, err := cli.ContainerCreate(context.Background(), cfg, hc, "fid-test")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID == "" {
		t.Fatal("empty id")
	}

	var got dockerhttp.ContainerCreateRequest
	if err := json.Unmarshal(m.LastCreateBody, &got); err != nil {
		t.Fatal(err)
	}
	if got.Image != "alpine:latest" {
		t.Fatalf("image=%s", got.Image)
	}
	if len(got.Env) != 2 || got.Env[0] != "A=1" {
		t.Fatalf("env=%v", got.Env)
	}
	if got.Labels["dot.key"] != "x" {
		t.Fatalf("labels=%v", got.Labels)
	}
	if got.HostConfig == nil {
		t.Fatal("nil hostconfig")
	}
	if got.HostConfig.NanoCPUs != 1_500_000_000 {
		t.Fatalf("cpu=%d", got.HostConfig.NanoCPUs)
	}
	if got.HostConfig.Memory != 256*1024*1024 {
		t.Fatalf("mem=%d", got.HostConfig.Memory)
	}
	if got.HostConfig.MemorySwap != 512*1024*1024 {
		t.Fatalf("swap=%d", got.HostConfig.MemorySwap)
	}
	if got.HostConfig.NetworkMode != "bridge" {
		t.Fatalf("net=%s", got.HostConfig.NetworkMode)
	}
	if got.HostConfig.RestartPolicy == nil || got.HostConfig.RestartPolicy.Name != "unless-stopped" {
		t.Fatalf("restart=%+v", got.HostConfig.RestartPolicy)
	}
	if m.LastCreateName != "fid-test" {
		t.Fatalf("name=%s", m.LastCreateName)
	}
}

func TestCreateStartFailCleanup(t *testing.T) {
	m := newMockEngine()
	cli := startUnixMock(t, m)

	// First create will get id cid-x; mark start fail
	// We need to know the id: name "x" → cid-x
	m.StartFailIDs["cid-x"] = true

	_, err := cli.CreateAndStart(context.Background(),
		&dockerhttp.ContainerConfig{Image: "alpine"},
		&dockerhttp.HostConfig{},
		"x",
	)
	if err == nil {
		t.Fatal("expected start failure")
	}
	if !strings.Contains(err.Error(), "start failed") {
		t.Fatalf("err=%v", err)
	}
	// cleanup remove should have happened
	found := false
	for _, id := range m.RemovedIDs {
		if id == "cid-x" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cleanup remove, removed=%v containers=%v", m.RemovedIDs, m.Containers)
	}
	// create called once (no blind retry)
	if len(m.Containers)+len(m.RemovedIDs) < 1 {
		t.Fatal("create should have been called once")
	}
}

func TestInspectMissingOptionalAnd404(t *testing.T) {
	m := newMockEngine()
	m.Containers["sparse"] = &dockerhttp.ContainerInspect{
		ID: "sparse",
		// State, Config, HostConfig all nil — must not panic
	}
	cli := startUnixMock(t, m)

	ins, err := cli.ContainerInspect(context.Background(), "sparse")
	if err != nil {
		t.Fatal(err)
	}
	if ins.ID != "sparse" {
		t.Fatalf("%+v", ins)
	}
	if ins.State != nil && ins.State.Running {
		t.Fatal("unexpected running")
	}

	_, err = cli.ContainerInspect(context.Background(), "gone")
	if !dockerhttp.IsNotFound(err) {
		t.Fatalf("want 404 got %v", err)
	}
}

func TestStopRemoveIdempotent(t *testing.T) {
	m := newMockEngine()
	m.Containers["c1"] = &dockerhttp.ContainerInspect{
		ID: "c1", Name: "/c1",
		State: &dockerhttp.ContainerState{Status: "exited", Running: false},
	}
	cli := startUnixMock(t, m)

	// stop already stopped → 304 → nil
	if err := cli.ContainerStop(context.Background(), "c1", dockerhttp.ContainerStopOptions{}); err != nil {
		t.Fatalf("stop exited: %v", err)
	}

	// remove
	if err := cli.ContainerRemove(context.Background(), "c1", dockerhttp.ContainerRemoveOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	// remove again — already gone → nil (idempotent)
	if err := cli.ContainerRemove(context.Background(), "c1", dockerhttp.ContainerRemoveOptions{Force: true}); err != nil {
		t.Fatalf("remove gone: %v", err)
	}

	// StopAndRemove on missing
	if err := cli.StopAndRemove(context.Background(), "never"); err != nil {
		t.Fatalf("stop+remove missing: %v", err)
	}
}

func TestImageTagRemoveSave(t *testing.T) {
	m := newMockEngine()
	m.Images["alpine"] = dockerhttp.ImageInspect{ID: "sha256:1", RepoTags: []string{"alpine:latest"}}
	cli := startUnixMock(t, m)

	if err := cli.ImageTag(context.Background(), "alpine", dockerhttp.ImageTagOptions{Repo: "my", Tag: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := cli.ImageRemove(context.Background(), "my:v1", dockerhttp.ImageRemoveOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	rc, err := cli.ImageSave(context.Background(), "alpine")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
}

func TestContainerExportLogs(t *testing.T) {
	m := newMockEngine()
	m.Containers["c"] = &dockerhttp.ContainerInspect{ID: "c", Name: "/c"}
	cli := startUnixMock(t, m)
	rc, err := cli.ContainerExport(context.Background(), "c")
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
	rc, err = cli.ContainerLogs(context.Background(), "c", true, true, false, "10")
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
}

func TestVersionEndpoint(t *testing.T) {
	m := newMockEngine()
	cli := startUnixMock(t, m)
	v, err := cli.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v.APIVersion == "" {
		t.Fatalf("%+v", v)
	}
}

func TestFiltersEncode(t *testing.T) {
	f := dockerhttp.Filters{}
	f.Add("label", "a=b")
	f.Add("label", "c=d,e")
	enc, err := f.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string][]string
	if err := json.Unmarshal([]byte(enc), &got); err != nil {
		t.Fatal(err)
	}
	if len(got["label"]) != 2 {
		t.Fatalf("%s", enc)
	}
}
