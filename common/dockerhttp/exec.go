package dockerhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ExecConfig is the body for POST /containers/{id}/exec.
type ExecConfig struct {
	User         string   `json:"User,omitempty"`
	Privileged   bool     `json:"Privileged,omitempty"`
	Tty          bool     `json:"Tty,omitempty"`
	AttachStdin  bool     `json:"AttachStdin,omitempty"`
	AttachStdout bool     `json:"AttachStdout,omitempty"`
	AttachStderr bool     `json:"AttachStderr,omitempty"`
	Detach       bool     `json:"Detach,omitempty"`
	DetachKeys   string   `json:"DetachKeys,omitempty"`
	Env          []string `json:"Env,omitempty"`
	WorkingDir   string   `json:"WorkingDir,omitempty"`
	Cmd          []string `json:"Cmd"`
}

// ExecCreateResponse is returned by exec create.
type ExecCreateResponse struct {
	ID string `json:"Id"`
}

// ExecStartOptions controls POST /exec/{id}/start.
type ExecStartOptions struct {
	Detach bool `json:"Detach"`
	Tty    bool `json:"Tty"`
}

// ExecInspect is a subset of GET /exec/{id}/json.
type ExecInspect struct {
	ID          string `json:"ID"`
	Running     bool   `json:"Running"`
	ExitCode    int    `json:"ExitCode"`
	Pid         int    `json:"Pid"`
	ContainerID string `json:"ContainerID"`
}

// ContainerExecCreate creates an exec instance in a running container.
func (c *Client) ContainerExecCreate(ctx context.Context, container string, cfg ExecConfig) (*ExecCreateResponse, error) {
	var out ExecCreateResponse
	p := "/containers/" + escapeName(container) + "/exec"
	if err := c.doJSON(ctx, http.MethodPost, p, nil, cfg, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ContainerExecStart starts an exec. When Detach is false, the response body
// may be a multiplexed hijack stream; caller must Close. For one-shot
// commands prefer ContainerExecRun.
func (c *Client) ContainerExecStart(ctx context.Context, execID string, opts ExecStartOptions) (io.ReadCloser, error) {
	b, err := json.Marshal(opts)
	if err != nil {
		return nil, err
	}
	p := "/exec/" + escapeName(execID) + "/start"
	resp, err := c.doRaw(ctx, http.MethodPost, p, nil, bytes.NewReader(b), "application/json")
	if err != nil {
		return nil, err
	}
	if opts.Detach {
		discardAndClose(resp)
		return nil, nil
	}
	return resp.Body, nil
}

// ContainerExecInspect inspects an exec instance.
func (c *Client) ContainerExecInspect(ctx context.Context, execID string) (*ExecInspect, error) {
	var out ExecInspect
	p := "/exec/" + escapeName(execID) + "/json"
	if err := c.doJSON(ctx, http.MethodGet, p, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ContainerExecRun creates, starts (attached), drains output, and returns the exit code.
func (c *Client) ContainerExecRun(ctx context.Context, container string, cmd []string) (exitCode int, err error) {
	created, err := c.ContainerExecCreate(ctx, container, ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	})
	if err != nil {
		return -1, err
	}
	rc, err := c.ContainerExecStart(ctx, created.ID, ExecStartOptions{Detach: false})
	if err != nil {
		return -1, err
	}
	if rc != nil {
		_, copyErr := io.Copy(io.Discard, rc)
		_ = rc.Close()
		if copyErr != nil {
			return -1, copyErr
		}
	}
	ins, err := c.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return -1, err
	}
	if ins.Running {
		return -1, fmt.Errorf("exec is still running after output closed")
	}
	return ins.ExitCode, nil
}
