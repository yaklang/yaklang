//go:build docker_integration

package dockerhttp_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

func liveClient(t *testing.T) (*dockerhttp.Client, context.Context) {
	t.Helper()
	c, err := dockerhttp.New(dockerhttp.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(func() { cancel(); c.Close() })
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Docker integration requires an available daemon: %v", err)
	}
	return c, ctx
}
func liveImage(t *testing.T, c *dockerhttp.Client, ctx context.Context) string {
	t.Helper()
	ref := os.Getenv("DOCKERHTTP_TEST_IMAGE")
	if ref == "" {
		ref = "alpine:3.20"
	}
	if _, found, err := c.ResolveImageID(ctx, ref); err != nil {
		t.Fatal(err)
	} else if !found {
		t.Fatalf("integration image %s must be preinstalled", ref)
	}
	return ref
}
func liveUnique(t *testing.T) string {
	return fmt.Sprintf("dockerhttp-test-%d-%s", time.Now().UnixNano(), strings.ToLower(t.Name()))
}
func cleanupContainer(t *testing.T, c *dockerhttp.Client, id string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := c.StopAndRemove(ctx, id); err != nil {
			t.Errorf("cleanup container %s: %v", id, err)
		}
	})
}
func cleanupImage(t *testing.T, c *dockerhttp.Client, ref string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := c.ImageRemove(ctx, ref, dockerhttp.ImageRemoveOptions{}); err != nil && !dockerhttp.IsNotFound(err) {
			t.Errorf("cleanup image %s: %v", ref, err)
		}
	})
}

func TestLivePullAndVersion(t *testing.T) {
	c, ctx := liveClient(t)
	ref := liveRegistry(t, c, ctx)
	if err := c.ImagePull(ctx, ref, dockerhttp.ImagePullOptions{}); err != nil {
		t.Fatal(err)
	}
	v, err := c.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Engine %s, API %s (client %s), host %s", v.Version, v.APIVersion, c.ClientVersion(), c.DaemonHost())
}
func TestLiveLifecycleExecAndNAT(t *testing.T) {
	c, ctx := liveClient(t)
	label := liveUnique(t)
	ins, err := c.CreateAndStart(ctx, &dockerhttp.ContainerConfig{
		Image: "nginx:alpine", Cmd: []string{"sh", "-c", "echo dockerhttp-ok > /usr/share/nginx/html/index.html; exec nginx -g 'daemon off;'"},
		Env: []string{"DOCKERHTTP_TEST=1"}, Labels: map[string]string{"yaklang.dockerhttp.test": label}, ExposedPorts: map[string]struct{}{"80/tcp": {}},
	}, &dockerhttp.HostConfig{Memory: 32 << 20, NanoCPUs: 500_000_000, PortBindings: dockerhttp.PortMap{"80/tcp": {{HostIP: "127.0.0.1", HostPort: "0"}}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	cleanupContainer(t, c, ins.ID)
	if ins.State == nil || !ins.State.Running || ins.HostConfig.Memory != 32<<20 || ins.HostConfig.NanoCPUs != 500_000_000 {
		t.Fatalf("bad inspect: %+v", ins)
	}
	if ins.Config.Labels["yaklang.dockerhttp.test"] != label {
		t.Fatal("label lost")
	}
	code, err := c.ContainerExecRun(ctx, ins.ID, []string{"sh", "-c", "test \"$DOCKERHTTP_TEST\" = 1; exit 7"})
	if err != nil || code != 7 {
		t.Fatalf("exec: %d %v", code, err)
	}
	found, err := c.FindContainerByLabel(ctx, "yaklang.dockerhttp.test", label)
	if err != nil || len(found) != 1 || found[0].ID != ins.ID {
		t.Fatalf("label lookup: %+v %v", found, err)
	}
	ports := ins.NetworkSettings.Ports["80/tcp"]
	if len(ports) != 1 || ports[0].HostPort == "" {
		t.Fatalf("no published port: %+v", ports)
	}
	client := &http.Client{Timeout: time.Second}
	url := "http://127.0.0.1:" + ports[0].HostPort
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, err := client.Get(url)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr == nil && strings.TrimSpace(string(body)) == "dockerhttp-ok" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("published HTTP service did not become ready: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	networks, err := c.NetworkList(ctx, dockerhttp.NetworkListOptions{})
	if err != nil || len(networks) == 0 {
		t.Fatalf("networks: %v", err)
	}
	if _, err := c.NetworkInspect(ctx, networks[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := c.StopAndRemove(ctx, ins.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.StopAndRemove(ctx, ins.ID); err != nil {
		t.Fatal(err)
	}
}
func TestLiveSaveLoadExportImport(t *testing.T) {
	c, ctx := liveClient(t)
	ref := liveImage(t, c, ctx)
	before, err := c.ImageInspect(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	repo := liveUnique(t)
	tag := repo + ":roundtrip"
	if err := c.ImageTag(ctx, ref, dockerhttp.ImageTagOptions{Repo: repo, Tag: "roundtrip"}); err != nil {
		t.Fatal(err)
	}
	cleanupImage(t, c, tag)
	rc, err := c.ImageSave(ctx, tag)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ImageRemove(ctx, tag, dockerhttp.ImageRemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := c.ImageLoad(ctx, bytes.NewReader(archive)); err != nil {
		t.Fatal(err)
	}
	after, err := c.ImageInspect(ctx, tag)
	if err != nil || after.ID != before.ID {
		t.Fatalf("image identity changed: %+v %v", after, err)
	}
	ins, err := c.CreateAndStart(ctx, &dockerhttp.ContainerConfig{Image: ref, Cmd: []string{"sleep", "120"}}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	cleanupContainer(t, c, ins.ID)
	rc, err = c.ContainerExport(ctx, ins.ID)
	if err != nil {
		t.Fatal(err)
	}
	imported := repo + ":imported"
	cleanupImage(t, c, imported)
	err = c.ImageImport(ctx, rc, dockerhttp.ImageImportOptions{Repository: imported})
	rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if id, found, err := c.ResolveImageID(ctx, imported); err != nil || !found || id == "" {
		t.Fatalf("imported identity: %s %v %v", id, found, err)
	}
}

// liveRegistry serves a tiny valid registry fixture through a temporary nginx
// container. The daemon pulls only from its own loopback, without Docker Hub.
// nginx:alpine must already be installed; no user images are removed or retagged.
func liveRegistry(t *testing.T, c *dockerhttp.Client, ctx context.Context) string {
	t.Helper()
	dir := t.TempDir()
	var layer bytes.Buffer
	gz := gzip.NewWriter(&layer)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "marker", Mode: 0644, Size: 2}); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("ok"))
	tw.Close()
	gz.Close()
	uncompressed, err := gzip.NewReader(bytes.NewReader(layer.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(uncompressed)
	uncompressed.Close()
	if err != nil {
		t.Fatal(err)
	}
	digest := func(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }
	version, err := c.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(map[string]any{"architecture": version.Arch, "os": "linux", "rootfs": map[string]any{"type": "layers", "diff_ids": []string{digest(raw)}}})
	if err != nil {
		t.Fatal(err)
	}
	media := "application/vnd.docker.distribution.manifest.v2+json"
	manifest, err := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": media, "config": map[string]any{"mediaType": "application/vnd.docker.container.image.v1+json", "size": len(config), "digest": digest(config)}, "layers": []any{map[string]any{"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip", "size": layer.Len(), "digest": digest(layer.Bytes())}}})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"v2/fixture/manifests/" + digest(manifest):  manifest,
		"v2/fixture/manifests/latest":               manifest,
		"v2/fixture/blobs/" + digest(config):        config,
		"v2/fixture/blobs/" + digest(layer.Bytes()): layer.Bytes(),
	} {
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	conf := `server { listen 80; root /registry; location = /v2/ { return 200 '{}'; } location / { default_type application/vnd.docker.distribution.manifest.v2+json; } }`
	if err := os.WriteFile(filepath.Join(dir, "registry.conf"), []byte(conf), 0644); err != nil {
		t.Fatal(err)
	}
	ins, err := c.CreateAndStart(ctx, &dockerhttp.ContainerConfig{Image: "nginx:alpine", ExposedPorts: map[string]struct{}{"80/tcp": {}}}, &dockerhttp.HostConfig{
		Mounts:       []dockerhttp.Mount{{Type: "bind", Source: dir, Target: "/registry", ReadOnly: true}, {Type: "bind", Source: filepath.Join(dir, "registry.conf"), Target: "/etc/nginx/conf.d/default.conf", ReadOnly: true}},
		PortBindings: dockerhttp.PortMap{"80/tcp": {{HostIP: "127.0.0.1", HostPort: "0"}}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	cleanupContainer(t, c, ins.ID)
	bindings := ins.NetworkSettings.Ports["80/tcp"]
	if len(bindings) != 1 {
		t.Fatal("registry port not published")
	}
	ref := "127.0.0.1:" + bindings[0].HostPort + "/fixture:latest"
	cleanupImage(t, c, ref)
	client := &http.Client{Timeout: time.Second}
	for deadline := time.Now().Add(5 * time.Second); ; {
		resp, err := client.Get("http://127.0.0.1:" + bindings[0].HostPort + "/v2/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("registry fixture did not start: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ref
}
