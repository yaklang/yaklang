package dockerhttp

import (
	"context"
	"net/http"
	"net/url"
)

// NetworkList lists networks.
func (c *Client) NetworkList(ctx context.Context, opts NetworkListOptions) ([]NetworkResource, error) {
	q := url.Values{}
	if err := setFilters(q, opts.Filters); err != nil {
		return nil, err
	}
	var out []NetworkResource
	if err := c.doJSON(ctx, http.MethodGet, "/networks", q, nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []NetworkResource{}
	}
	return out, nil
}

// NetworkInspect inspects a network by id or name.
func (c *Client) NetworkInspect(ctx context.Context, idOrName string) (*NetworkResource, error) {
	var out NetworkResource
	p := "/networks/" + escapeName(idOrName)
	if err := c.doJSON(ctx, http.MethodGet, p, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// NetworkConnect connects a container to a network.
func (c *Client) NetworkConnect(ctx context.Context, networkID, containerID string, ep *EndpointSettings) error {
	body := map[string]any{
		"Container": containerID,
	}
	if ep != nil {
		body["EndpointConfig"] = ep
	}
	p := "/networks/" + escapeName(networkID) + "/connect"
	return c.doJSON(ctx, http.MethodPost, p, nil, body, nil)
}

// NetworkDisconnect disconnects a container from a network.
func (c *Client) NetworkDisconnect(ctx context.Context, networkID, containerID string, force bool) error {
	body := map[string]any{
		"Container": containerID,
		"Force":     force,
	}
	p := "/networks/" + escapeName(networkID) + "/disconnect"
	return c.doJSON(ctx, http.MethodPost, p, nil, body, nil)
}

// NetworkListOptions controls network listing.
type NetworkListOptions struct {
	Filters Filters
}

// NetworkResource is a subset of Engine network JSON.
type NetworkResource struct {
	Name       string                      `json:"Name"`
	ID         string                      `json:"Id"`
	Driver     string                      `json:"Driver"`
	Scope      string                      `json:"Scope"`
	Internal   bool                        `json:"Internal"`
	Attachable bool                        `json:"Attachable"`
	Ingress    bool                        `json:"Ingress"`
	Containers map[string]EndpointResource `json:"Containers"`
	Labels     map[string]string           `json:"Labels"`
}

// EndpointResource is per-container info on a network.
type EndpointResource struct {
	Name        string `json:"Name"`
	EndpointID  string `json:"EndpointID"`
	MacAddress  string `json:"MacAddress"`
	IPv4Address string `json:"IPv4Address"`
	IPv6Address string `json:"IPv6Address"`
}
