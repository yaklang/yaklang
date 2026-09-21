package dockerhttp

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client is a lightweight Docker Engine HTTP API client.
// It does not import github.com/docker/docker or gogo/protobuf.
type Client struct {
	host       *Host
	httpClient *http.Client
	transport  *http.Transport
	tlsConfig  *tls.Config
	timeout    time.Duration
	scheme     string
	reqHost    string

	version          string // negotiated / forced API version (without "v")
	manualOverride   bool
	negotiateVersion bool
	negotiated       bool
	negotiateMu      sync.Mutex

	userAgent string
}

// Opt configures a Client.
type Opt func(*Client) error

// New creates a Client. By default it uses DefaultDockerHost, MaxAPIVersion,
// and enables API version negotiation on the first request.
func New(opts ...Opt) (*Client, error) {
	h, err := ParseHostURL(DefaultDockerHost)
	if err != nil {
		return nil, err
	}
	c := &Client{
		host:             h,
		version:          MaxAPIVersion,
		negotiateVersion: true,
		userAgent:        "yaklang-dockerhttp/1.0",
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	if c.httpClient == nil {
		hc, tr, err := newHTTPClient(c.host, c.tlsConfig, c.timeout)
		if err != nil {
			return nil, err
		}
		c.httpClient = hc
		c.transport = tr
	}
	c.scheme = httpScheme(c.host, c.tlsConfig)
	c.reqHost = requestHost(c.host)
	return c, nil
}

// FromEnv applies DOCKER_HOST, DOCKER_API_VERSION, and optional
// DOCKER_TLS_VERIFY + DOCKER_CERT_PATH (TCP TLS clients).
func FromEnv(c *Client) error {
	if err := WithHost(HostFromEnv())(c); err != nil {
		return err
	}
	if v := APIVersionFromEnv(); v != "" {
		if err := WithVersion(v)(c); err != nil {
			return err
		}
	}
	return WithTLSFromEnv()(c)
}

// WithHost sets the Docker host URL.
func WithHost(host string) Opt {
	return func(c *Client) error {
		h, err := ParseHostURL(host)
		if err != nil {
			return err
		}
		c.host = h
		return nil
	}
}

// WithVersion forces an API version and disables negotiation.
func WithVersion(version string) Opt {
	return func(c *Client) error {
		if version == "" {
			return nil
		}
		version = strings.TrimPrefix(version, "v")
		if !validAPIVersion(version) {
			return fmt.Errorf("invalid Docker API version %q", version)
		}
		c.version = version
		c.manualOverride = true
		c.negotiateVersion = false
		return nil
	}
}

// WithAPIVersionNegotiation enables lazy negotiation (default on).
func WithAPIVersionNegotiation() Opt {
	return func(c *Client) error {
		if !c.manualOverride {
			c.negotiateVersion = true
		}
		return nil
	}
}

// WithHTTPClient replaces the underlying HTTP client (for tests).
func WithHTTPClient(hc *http.Client) Opt {
	return func(c *Client) error {
		if hc == nil {
			return fmt.Errorf("nil HTTP client")
		}
		c.httpClient = hc
		c.transport = nil
		return nil
	}
}

// WithTLSConfig configures TLS for the daemon connection.
func WithTLSConfig(tlsConfig *tls.Config) Opt {
	return func(c *Client) error {
		c.tlsConfig = tlsConfig
		return nil
	}
}

// WithTimeout sets a client-wide HTTP timeout (prefer context instead).
func WithTimeout(d time.Duration) Opt {
	return func(c *Client) error {
		c.timeout = d
		return nil
	}
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Opt {
	return func(c *Client) error {
		c.userAgent = ua
		return nil
	}
}

// DaemonHost returns the configured host string.
func (c *Client) DaemonHost() string {
	return c.host.Raw
}

// ClientVersion returns the API version currently in use.
func (c *Client) ClientVersion() string {
	c.negotiateMu.Lock()
	defer c.negotiateMu.Unlock()
	return c.version
}

// Close closes idle connections.
func (c *Client) Close() error {
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	} else if c.httpClient != nil {
		if tr, ok := c.httpClient.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
	return nil
}

// HTTPClient returns the underlying *http.Client (shared; do not Close independently).
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

func (c *Client) setNegotiatedVersion(serverAPI string) error {
	c.negotiateMu.Lock()
	defer c.negotiateMu.Unlock()
	if c.manualOverride || c.negotiated {
		return nil
	}
	if serverAPI == "" {
		serverAPI = FallbackAPIVersion
	}
	if !validAPIVersion(serverAPI) {
		return &VersionIncompatibleError{Client: c.version, Server: serverAPI, Detail: "invalid API-Version header"}
	}
	if c.version == "" {
		c.version = MaxAPIVersion
	}
	if versionLessThan(serverAPI, c.version) {
		c.version = serverAPI
	}
	if c.negotiateVersion {
		c.negotiated = true
	}
	return nil
}

// versionLessThan reports whether a < b for "major.minor" versions.
func versionLessThan(a, b string) bool {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	var am, an, bm, bn int
	fmt.Sscanf(a, "%d.%d", &am, &an)
	fmt.Sscanf(b, "%d.%d", &bm, &bn)
	if am != bm {
		return am < bm
	}
	return an < bn
}

func validAPIVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
		}
		if _, err := strconv.ParseUint(part, 10, 16); err != nil {
			return false
		}
	}
	return true
}
