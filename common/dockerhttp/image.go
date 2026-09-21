package dockerhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ImageList lists images.
func (c *Client) ImageList(ctx context.Context, opts ImageListOptions) ([]ImageSummary, error) {
	q := url.Values{}
	if opts.All {
		q.Set("all", "1")
	}
	if err := setFilters(q, opts.Filters); err != nil {
		return nil, err
	}
	var out []ImageSummary
	if err := c.doJSON(ctx, http.MethodGet, "/images/json", q, nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []ImageSummary{}
	}
	return out, nil
}

// ImageInspect inspects an image by name/id/digest.
func (c *Client) ImageInspect(ctx context.Context, ref string) (*ImageInspect, error) {
	var out ImageInspect
	p := "/images/" + escapeName(ref) + "/json"
	if err := c.doJSON(ctx, http.MethodGet, p, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResolveImageID resolves a name/tag/digest to an image ID.
// ok=false means not found (404); other errors are returned as err.
func (c *Client) ResolveImageID(ctx context.Context, ref string) (id string, ok bool, err error) {
	img, err := c.ImageInspect(ctx, ref)
	if err != nil {
		if IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if img.ID == "" {
		return "", false, nil
	}
	return img.ID, true, nil
}

// ImageTag tags an image.
func (c *Client) ImageTag(ctx context.Context, source string, opts ImageTagOptions) error {
	q := url.Values{}
	q.Set("repo", opts.Repo)
	if opts.Tag != "" {
		q.Set("tag", opts.Tag)
	}
	p := "/images/" + escapeName(source) + "/tag"
	return c.doJSON(ctx, http.MethodPost, p, q, nil, nil)
}

// ImageRemove removes an image.
func (c *Client) ImageRemove(ctx context.Context, ref string, opts ImageRemoveOptions) error {
	q := url.Values{}
	if opts.Force {
		q.Set("force", "1")
	}
	if opts.NoPrune {
		q.Set("noprune", "1")
	}
	p := "/images/" + escapeName(ref)
	return c.doJSON(ctx, http.MethodDelete, p, q, nil, nil)
}

// ImagePullOptions controls ImagePull.
type ImagePullOptions struct {
	// Platform selects a platform (e.g. "linux/amd64") when the daemon supports it.
	Platform string
	// RegistryAuth is an optional base64url-encoded AuthConfig JSON for
	// X-Registry-Auth. Empty is fine for anonymous public pulls.
	RegistryAuth string
	// All is unused for single-ref pull; reserved for symmetry.
	All bool
}

// ImagePull pulls an image reference (name[:tag|@digest]) and consumes the
// Engine JSON progress stream. Stream error events are surfaced as errors.
// Anonymous pull is sufficient for public images such as alpine/busybox.
func (c *Client) ImagePull(ctx context.Context, ref string, opts ImagePullOptions) error {
	fromImage, tag := splitImageRef(ref)
	q := url.Values{}
	q.Set("fromImage", fromImage)
	if tag != "" {
		q.Set("tag", tag)
	}
	if opts.Platform != "" {
		q.Set("platform", opts.Platform)
	}

	if err := c.ensureNegotiated(ctx); err != nil {
		return err
	}
	req, err := c.newRequest(ctx, http.MethodPost, c.apiPath("/images/create", q), nil, "")
	if err != nil {
		return err
	}
	if opts.RegistryAuth != "" {
		req.Header.Set("X-Registry-Auth", opts.RegistryAuth)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseErrorBody(resp)
	}
	return consumeJSONMessageStream(ctx, resp.Body)
}

// ImageImportOptions controls ImageImport ("docker import").
// Note: Engine "import" creates a *new image* from a filesystem tarball
// (or URL). Container filesystem export → ImageImport is the usual
// "container import" roundtrip. ImageLoad restores a full image archive
// from ImageSave (layers + manifests).
type ImageImportOptions struct {
	// Source is "-" to read from r, or an HTTP(S) URL the daemon fetches.
	Source string
	// Repository is the optional name to apply (repo[:tag]).
	Repository string
	// Message is an optional commit message.
	Message string
	// Changes are Dockerfile-like instructions applied on import.
	Changes  []string
	Platform string
}

// ImageImport creates an image from a tarball reader or URL (POST /images/create?fromSrc=...).
// When Source is "-" or empty, the tarball is read from r.
func (c *Client) ImageImport(ctx context.Context, r io.Reader, opts ImageImportOptions) error {
	src := opts.Source
	if src == "" {
		src = "-"
	}
	q := url.Values{}
	q.Set("fromSrc", src)
	if opts.Repository != "" {
		q.Set("repo", opts.Repository)
	}
	if opts.Message != "" {
		q.Set("message", opts.Message)
	}
	if opts.Platform != "" {
		q.Set("platform", opts.Platform)
	}
	for _, ch := range opts.Changes {
		q.Add("changes", ch)
	}

	var body io.Reader
	var ct string
	if src == "-" {
		if r == nil {
			return fmt.Errorf("ImageImport: reader required when fromSrc=-")
		}
		body = r
		ct = "application/x-tar"
	}

	resp, err := c.doRaw(ctx, http.MethodPost, "/images/create", q, body, ct)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return consumeJSONMessageStream(ctx, resp.Body)
}

// ImageLoad loads an image archive from r. It does NOT unconditionally retry.
// HTTP 200 with error events in the JSON stream is treated as failure.
func (c *Client) ImageLoad(ctx context.Context, r io.Reader) error {
	resp, err := c.doRaw(ctx, http.MethodPost, "/images/load", nil, r, "application/x-tar")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return consumeJSONMessageStream(ctx, resp.Body)
}

// ImageSave exports images as a tar stream. Caller must Close the ReadCloser.
func (c *Client) ImageSave(ctx context.Context, refs ...string) (io.ReadCloser, error) {
	q := url.Values{}
	for _, ref := range refs {
		q.Add("names", ref)
	}
	resp, err := c.doRaw(ctx, http.MethodGet, "/images/get", q, nil, "")
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// splitImageRef splits "repo:tag" or "repo@digest" into fromImage + tag query values.
func splitImageRef(ref string) (fromImage, tag string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", ""
	}
	if i := strings.LastIndex(ref, "@"); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	i := strings.LastIndex(ref, ":")
	if i < 0 {
		return ref, "latest"
	}
	// registry:port/name without tag → treat whole ref as fromImage.
	rest := ref[i+1:]
	if strings.Contains(rest, "/") {
		return ref, "latest"
	}
	return ref[:i], rest
}

// consumeJSONMessageStream reads newline-delimited JSONMessage objects and
// returns the first error event, or ctx cancellation.
func consumeJSONMessageStream(ctx context.Context, r io.Reader) error {
	decoder := json.NewDecoder(r)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var msg JSONMessage
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decode image stream: %w", err)
		}
		if err := msg.Err(); err != nil {
			return err
		}
	}
}
