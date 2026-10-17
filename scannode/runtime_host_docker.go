package scannode

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/dockerhttp"
)

const runtimeHostContainerCAPath = "/etc/legion/nats-ca.pem"

const runtimeHostCleanupLabel = "legion.ai.runtime.cleanup_key"

type runtimeHostContainer struct {
	ID              string
	ImageID         string
	Running         bool
	Labels          map[string]string
	CPUMillicores   uint64
	MemoryBytes     uint64
	MemorySwapBytes uint64
}

type runtimeHostContainerInput struct {
	NATSCAFile      string
	Name            string
	Image           string
	Network         string
	Args            []string
	Env             []string
	Labels          map[string]string
	CPUMillicores   uint64
	MemoryBytes     uint64
	MemorySwapBytes uint64
}

type runtimeHostDocker interface {
	Ping(context.Context) error
	ResolveImageID(context.Context, string) (string, bool, error)
	LoadImage(context.Context, io.Reader) error
	FindContainer(context.Context, string) (runtimeHostContainer, bool, error)
	CreateAndStart(context.Context, runtimeHostContainerInput) (runtimeHostContainer, error)
	Inspect(context.Context, string) (runtimeHostContainer, bool, error)
	StopAndRemove(context.Context, string) error
	Close() error
}

type localRuntimeHostDocker struct {
	client *dockerhttp.Client
}

func newLocalRuntimeHostDocker() (*localRuntimeHostDocker, error) {
	cli, err := dockerhttp.New(
		dockerhttp.FromEnv,
		dockerhttp.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}
	return &localRuntimeHostDocker{client: cli}, nil
}

func (d *localRuntimeHostDocker) Ping(ctx context.Context) error {
	return d.client.Ping(ctx)
}

func (d *localRuntimeHostDocker) ResolveImageID(ctx context.Context, selector string) (string, bool, error) {
	image, err := d.client.ImageInspect(ctx, strings.TrimSpace(selector))
	if err != nil {
		if dockerhttp.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if strings.TrimSpace(image.ID) == "" {
		return "", false, fmt.Errorf("runtime image has no local identity")
	}
	return strings.TrimSpace(image.ID), true, nil
}

func (d *localRuntimeHostDocker) LoadImage(ctx context.Context, archive io.Reader) error {
	return d.client.ImageLoad(ctx, archive)
}

func (d *localRuntimeHostDocker) FindContainer(ctx context.Context, cleanupKey string) (runtimeHostContainer, bool, error) {
	items, err := d.client.ContainerList(ctx, dockerhttp.ContainerListOptions{
		All:     true,
		Filters: dockerhttp.Filters{"label": {runtimeHostCleanupLabel + "=" + cleanupKey}},
	})
	if err != nil {
		return runtimeHostContainer{}, false, err
	}
	if len(items) == 0 {
		return runtimeHostContainer{}, false, nil
	}
	if len(items) > 1 {
		return runtimeHostContainer{}, false, fmt.Errorf("multiple containers use the same cleanup key")
	}
	return d.Inspect(ctx, items[0].ID)
}

func (d *localRuntimeHostDocker) CreateAndStart(ctx context.Context, input runtimeHostContainerInput) (runtimeHostContainer, error) {
	if input.CPUMillicores > math.MaxInt64/1_000_000 || input.MemoryBytes > math.MaxInt64 || input.MemorySwapBytes > math.MaxInt64 {
		return runtimeHostContainer{}, fmt.Errorf("runtime container resource limit overflows int64")
	}
	if err := validateRuntimeHostCAFile(input.NATSCAFile); err != nil {
		return runtimeHostContainer{}, err
	}
	var binds []string
	if input.NATSCAFile != "" {
		binds = []string{input.NATSCAFile + ":" + runtimeHostContainerCAPath + ":ro"}
	}
	info, err := d.client.CreateAndStart(ctx, &dockerhttp.ContainerConfig{
		Image: input.Image, Env: input.Env, Cmd: input.Args, Labels: input.Labels,
	}, &dockerhttp.HostConfig{
		NetworkMode:   input.Network,
		Binds:         binds,
		RestartPolicy: &dockerhttp.RestartPolicy{Name: "unless-stopped"},
		NanoCPUs:      int64(input.CPUMillicores * 1_000_000),
		Memory:        int64(input.MemoryBytes), MemorySwap: int64(input.MemorySwapBytes),
	}, input.Name)
	if err != nil {
		return runtimeHostContainer{}, err
	}
	return runtimeHostContainerFromInspect(info), nil
}

func (d *localRuntimeHostDocker) Inspect(ctx context.Context, containerID string) (runtimeHostContainer, bool, error) {
	info, err := d.client.ContainerInspect(ctx, strings.TrimSpace(containerID))
	if err != nil {
		if dockerhttp.IsNotFound(err) {
			return runtimeHostContainer{}, false, nil
		}
		return runtimeHostContainer{}, false, err
	}
	return runtimeHostContainerFromInspect(info), true, nil
}

func runtimeHostContainerFromInspect(info *dockerhttp.ContainerInspect) runtimeHostContainer {
	result := runtimeHostContainer{
		ID:      info.ID,
		ImageID: strings.TrimSpace(info.Image),
		Running: info.State != nil && info.State.Running,
	}
	if info.Config != nil {
		result.Labels = cloneStringMapValue(info.Config.Labels)
	}
	if info.HostConfig != nil {
		if info.HostConfig.NanoCPUs > 0 {
			result.CPUMillicores = uint64(info.HostConfig.NanoCPUs) / 1_000_000
		}
		if info.HostConfig.Memory > 0 {
			result.MemoryBytes = uint64(info.HostConfig.Memory)
		}
		if info.HostConfig.MemorySwap > 0 {
			result.MemorySwapBytes = uint64(info.HostConfig.MemorySwap)
		}
	}
	return result
}

func (d *localRuntimeHostDocker) StopAndRemove(ctx context.Context, containerID string) error {
	return d.client.StopAndRemove(ctx, containerID)
}

func (d *localRuntimeHostDocker) Close() error { return d.client.Close() }

func cloneStringMapValue(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

// Only public certificates from startup configuration can enter a runtime.
func validateRuntimeHostCAFile(path string) error {
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) || strings.ContainsAny(path, ":\r\n") {
		return fmt.Errorf("runtime CA requires a safe absolute path")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return fmt.Errorf("runtime CA must be an existing public certificate file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("runtime CA file is unreadable")
	}
	certificates := 0
	for len(strings.TrimSpace(string(data))) > 0 {
		if !strings.HasPrefix(strings.TrimSpace(string(data)), "-----BEGIN CERTIFICATE-----") {
			return fmt.Errorf("runtime CA file may contain only public certificates")
		}
		block, rest := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" {
			return fmt.Errorf("runtime CA file may contain only public certificates")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return fmt.Errorf("runtime CA certificate is invalid")
		}
		certificates++
		data = rest
	}
	if certificates == 0 {
		return fmt.Errorf("runtime CA file contains no certificates")
	}
	return nil
}
