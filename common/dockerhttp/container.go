package dockerhttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ContainerList lists containers.
func (c *Client) ContainerList(ctx context.Context, opts ContainerListOptions) ([]ContainerSummary, error) {
	q := url.Values{}
	if opts.All {
		q.Set("all", "1")
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Size {
		q.Set("size", "1")
	}
	if err := setFilters(q, opts.Filters); err != nil {
		return nil, err
	}
	var out []ContainerSummary
	if err := c.doJSON(ctx, http.MethodGet, "/containers/json", q, nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []ContainerSummary{}
	}
	return out, nil
}

// ContainerInspect inspects a container.
func (c *Client) ContainerInspect(ctx context.Context, id string) (*ContainerInspect, error) {
	var out ContainerInspect
	p := "/containers/" + escapeName(id) + "/json"
	if err := c.doJSON(ctx, http.MethodGet, p, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ContainerCreate creates a container. It does NOT retry on failure.
func (c *Client) ContainerCreate(ctx context.Context, config *ContainerConfig, host *HostConfig, name string) (*ContainerCreateResponse, error) {
	body := &ContainerCreateRequest{
		ContainerConfig: config,
		HostConfig:      host,
	}
	q := url.Values{}
	if name != "" {
		q.Set("name", name)
	}
	var out ContainerCreateResponse
	if err := c.doJSON(ctx, http.MethodPost, "/containers/create", q, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ContainerStart starts a container.
func (c *Client) ContainerStart(ctx context.Context, id string) error {
	p := "/containers/" + escapeName(id) + "/start"
	if err := c.ensureNegotiated(ctx); err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodPost, c.apiPath(p, nil), nil, "")
	if err != nil {
		return err
	}
	defer discardAndClose(resp)
	if resp.StatusCode == http.StatusNotModified || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	return parseErrorBody(resp)
}

// ContainerStop stops a container. Already-stopped is treated as success (304).
func (c *Client) ContainerStop(ctx context.Context, id string, opts ContainerStopOptions) error {
	q := url.Values{}
	if opts.Timeout != nil {
		q.Set("t", strconv.Itoa(*opts.Timeout))
	}
	p := "/containers/" + escapeName(id) + "/stop"
	if err := c.ensureNegotiated(ctx); err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodPost, c.apiPath(p, q), nil, "")
	if err != nil {
		return err
	}
	defer discardAndClose(resp)
	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotModified:
		return nil
	case http.StatusNotFound:
		return parseErrorBody(resp)
	default:
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		return parseErrorBody(resp)
	}
}

// ContainerRestart restarts a container.
func (c *Client) ContainerRestart(ctx context.Context, id string, opts ContainerStopOptions) error {
	q := url.Values{}
	if opts.Timeout != nil {
		q.Set("t", strconv.Itoa(*opts.Timeout))
	}
	p := "/containers/" + escapeName(id) + "/restart"
	return c.doJSON(ctx, http.MethodPost, p, q, nil, nil)
}

// ContainerKill kills a container.
func (c *Client) ContainerKill(ctx context.Context, id string, opts ContainerKillOptions) error {
	q := url.Values{}
	if opts.Signal != "" {
		q.Set("signal", opts.Signal)
	}
	p := "/containers/" + escapeName(id) + "/kill"
	return c.doJSON(ctx, http.MethodPost, p, q, nil, nil)
}

// ContainerRemove removes a container.
// 404 (already gone) is treated as success for idempotency.
func (c *Client) ContainerRemove(ctx context.Context, id string, opts ContainerRemoveOptions) error {
	q := url.Values{}
	if opts.Force {
		q.Set("force", "1")
	}
	if opts.RemoveVolumes {
		q.Set("v", "1")
	}
	if opts.RemoveLinks {
		q.Set("link", "1")
	}
	p := "/containers/" + escapeName(id)
	if err := c.ensureNegotiated(ctx); err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodDelete, c.apiPath(p, q), nil, "")
	if err != nil {
		return err
	}
	defer discardAndClose(resp)
	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return nil
	default:
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		return parseErrorBody(resp)
	}
}

// ContainerExport exports a container filesystem as a tar stream.
// Caller must Close the ReadCloser.
func (c *Client) ContainerExport(ctx context.Context, id string) (io.ReadCloser, error) {
	p := "/containers/" + escapeName(id) + "/export"
	resp, err := c.doRaw(ctx, http.MethodGet, p, nil, nil, "")
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// ContainerLogs returns a log stream. Caller must Close.
func (c *Client) ContainerLogs(ctx context.Context, id string, stdout, stderr, follow bool, tail string) (io.ReadCloser, error) {
	q := url.Values{}
	if stdout {
		q.Set("stdout", "1")
	}
	if stderr {
		q.Set("stderr", "1")
	}
	if follow {
		q.Set("follow", "1")
	}
	if tail != "" {
		q.Set("tail", tail)
	}
	p := "/containers/" + escapeName(id) + "/logs"
	resp, err := c.doRaw(ctx, http.MethodGet, p, q, nil, "")
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// CreateAndStart creates then starts a container. If start fails, it attempts
// to remove the created container (cleanup is best-effort and observable via
// the returned error wrapping). Create is never blindly retried.
func (c *Client) CreateAndStart(ctx context.Context, config *ContainerConfig, host *HostConfig, name string) (*ContainerInspect, error) {
	created, err := c.ContainerCreate(ctx, config, host, name)
	if err != nil {
		return nil, err
	}
	if created.ID == "" {
		return nil, fmt.Errorf("created container has no identity")
	}
	rollback := func(cause error) (*ContainerInspect, error) {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := c.ContainerRemove(cleanupCtx, created.ID, ContainerRemoveOptions{Force: true}); err != nil {
			return nil, fmt.Errorf("%w (cleanup remove: %v)", cause, err)
		}
		return nil, fmt.Errorf("%w (container removed)", cause)
	}
	if err := c.ContainerStart(ctx, created.ID); err != nil {
		return rollback(fmt.Errorf("start failed: %w", err))
	}
	ins, err := c.ContainerInspect(ctx, created.ID)
	if err != nil {
		return rollback(fmt.Errorf("started but inspect failed: %w", err))
	}
	if ins.ID == "" {
		return rollback(fmt.Errorf("inspected container has no identity"))
	}

	return ins, nil
}

// StopAndRemove stops (ignoring already-stopped) then removes a container.
func (c *Client) StopAndRemove(ctx context.Context, id string) error {
	_ = c.ContainerStop(ctx, id, ContainerStopOptions{}) // 304/404 handled
	return c.ContainerRemove(ctx, id, ContainerRemoveOptions{Force: true})
}

// FindContainerByLabel lists all containers matching a single label filter.
func (c *Client) FindContainerByLabel(ctx context.Context, key, value string) ([]ContainerSummary, error) {
	f := Filters{}
	f.Add("label", key+"="+value)
	return c.ContainerList(ctx, ContainerListOptions{All: true, Filters: f})
}
