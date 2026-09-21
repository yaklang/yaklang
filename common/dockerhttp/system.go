package dockerhttp

import (
	"context"
	"net/http"
)

// Ping hits /_ping (unversioned) and triggers version negotiation side-effects.
func (c *Client) Ping(ctx context.Context) error {
	info, err := c.pingRaw(ctx)
	if err != nil {
		return err
	}
	return c.setNegotiatedVersion(info.APIVersion)
}

// PingInfo returns ping headers and updates negotiation state.
func (c *Client) PingInfo(ctx context.Context) (PingInfo, error) {
	info, err := c.pingRaw(ctx)
	if err != nil {
		return PingInfo{}, err
	}
	return info, c.setNegotiatedVersion(info.APIVersion)
}

func (c *Client) pingRaw(ctx context.Context) (PingInfo, error) {
	path := c.unversionedPath("/_ping")

	resp, err := c.do(ctx, http.MethodHead, path, nil, "")
	if err == nil {
		info, perr := parsePingResp(resp)
		if perr == nil {
			return info, nil
		}
	}

	resp, err = c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return PingInfo{}, err
	}
	return parsePingResp(resp)
}

func parsePingResp(resp *http.Response) (PingInfo, error) {
	var info PingInfo
	defer discardAndClose(resp)
	info.APIVersion = resp.Header.Get("API-Version")
	info.OSType = resp.Header.Get("OSType")
	info.Experimental = resp.Header.Get("Docker-Experimental") == "true"
	switch resp.StatusCode {
	case http.StatusOK, http.StatusInternalServerError:
		return info, nil
	default:
		return info, &APIError{StatusCode: resp.StatusCode, Message: resp.Status}
	}
}

// Version returns GET /version.
func (c *Client) Version(ctx context.Context) (*VersionInfo, error) {
	var out VersionInfo
	if err := c.doJSON(ctx, http.MethodGet, "/version", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// NegotiateAPIVersion forces an immediate negotiation round-trip.
func (c *Client) NegotiateAPIVersion(ctx context.Context) error {
	if c.manualOverride {
		return nil
	}
	info, err := c.pingRaw(ctx)
	if err != nil {
		return err
	}
	if info.APIVersion == "" {
		return &VersionIncompatibleError{
			Client: c.ClientVersion(),
			Server: "",
			Detail: "API version negotiation failed: empty API-Version header",
		}
	}
	return c.setNegotiatedVersion(info.APIVersion)
}
