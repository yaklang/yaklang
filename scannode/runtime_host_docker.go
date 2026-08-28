package scannode

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

const runtimeHostDockerSocket = "unix:///var/run/docker.sock"

type runtimeHostContainer struct {
	ID      string
	ImageID string
	Running bool
	Labels  map[string]string
}

type runtimeHostContainerInput struct {
	Name    string
	Image   string
	Network string
	Args    []string
	Env     []string
	Labels  map[string]string
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
	client *client.Client
}

func newLocalRuntimeHostDocker() (*localRuntimeHostDocker, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(runtimeHostDockerSocket),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}
	return &localRuntimeHostDocker{client: cli}, nil
}

func (d *localRuntimeHostDocker) Ping(ctx context.Context) error {
	_, err := d.client.Ping(ctx)
	return err
}

func (d *localRuntimeHostDocker) ResolveImageID(ctx context.Context, selector string) (string, bool, error) {
	image, _, err := d.client.ImageInspectWithRaw(ctx, strings.TrimSpace(selector))
	if err != nil {
		if client.IsErrNotFound(err) {
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
	response, err := d.client.ImageLoad(ctx, archive, true)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, err = io.Copy(io.Discard, response.Body)
	return err
}

func (d *localRuntimeHostDocker) FindContainer(ctx context.Context, cleanupKey string) (runtimeHostContainer, bool, error) {
	items, err := d.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", runtimeHostCleanupLabel+"="+cleanupKey)),
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
	response, err := d.client.ContainerCreate(ctx, &container.Config{
		Image:  input.Image,
		Env:    input.Env,
		Cmd:    input.Args,
		Labels: input.Labels,
	}, &container.HostConfig{
		NetworkMode:   container.NetworkMode(input.Network),
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
	}, nil, nil, input.Name)
	if err != nil {
		return runtimeHostContainer{}, err
	}
	if err := d.client.ContainerStart(ctx, response.ID, container.StartOptions{}); err != nil {
		_ = d.client.ContainerRemove(ctx, response.ID, container.RemoveOptions{Force: true})
		return runtimeHostContainer{}, err
	}
	result, found, err := d.Inspect(ctx, response.ID)
	if err != nil {
		return runtimeHostContainer{}, err
	}
	if !found {
		return runtimeHostContainer{}, fmt.Errorf("created runtime container disappeared")
	}
	return result, nil
}

func (d *localRuntimeHostDocker) Inspect(ctx context.Context, containerID string) (runtimeHostContainer, bool, error) {
	info, err := d.client.ContainerInspect(ctx, strings.TrimSpace(containerID))
	if err != nil {
		if client.IsErrNotFound(err) {
			return runtimeHostContainer{}, false, nil
		}
		return runtimeHostContainer{}, false, err
	}
	return runtimeHostContainer{
		ID:      info.ID,
		ImageID: strings.TrimSpace(info.Image),
		Running: info.State != nil && info.State.Running,
		Labels:  cloneStringMapValue(info.Config.Labels),
	}, true, nil
}

func (d *localRuntimeHostDocker) StopAndRemove(ctx context.Context, containerID string) error {
	_ = d.client.ContainerStop(ctx, containerID, container.StopOptions{})
	err := d.client.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	if client.IsErrNotFound(err) {
		return nil
	}
	return err
}

func (d *localRuntimeHostDocker) Close() error { return d.client.Close() }

func cloneStringMapValue(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
