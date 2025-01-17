package yakdocker

import (
	"context"
	"github.com/docker/docker/client"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"net/url"
	"os"
	"time"
)

/*
yakdocker is lib for yaklang
*/

// Config is the config for yakdocker
type Config struct {
	Host    string
	Proxy   []string
	Context context.Context
	Cancel  context.CancelFunc

	Timeout time.Duration
}

func NewConfig(opt ...Option) *Config {
	c := &Config{
		Host:    client.DefaultDockerHost,
		Timeout: 10 * time.Second,
	}
	c.Context, c.Cancel = context.WithCancel(context.Background())
	for _, o := range opt {
		o(c)
	}
	return c
}

func (c *Config) GetDockerClient() (*client.Client, error) {
	if c.Host == "" {
		WithHostFromEnv()(c)
	}
	var opts []client.Opt
	var schema string
	if c.Host != "" {
		opts = append(opts, client.WithHost(c.Host))
		u, _ := url.Parse(c.Host)
		if u != nil {
			schema = u.Scheme
		}
	}
	opts = append(opts, client.WithDialContext(func(ctx context.Context, network, addr string) (net.Conn, error) {
		if schema == "unix" {
			u, err := url.Parse(c.Host)
			if err != nil {
				return nil, err
			}
			return net.DialTimeout(schema, u.Path, c.Timeout)
		}

		if schema == "npipe" {
			return nil, utils.Errorf("(%v) npipe not support", c.Host)
		}
		return netx.DialContext(ctx, addr, c.Proxy...)
	}))
	opts = append(opts, client.WithAPIVersionNegotiation())
	return client.NewClientWithOpts(opts...)
}

type Option func(*Config)

func WithHost(host string) Option {
	return func(c *Config) {
		c.Host = host
	}
}

func WithHostFromEnv() Option {
	return func(c *Config) {
		if h := os.Getenv(client.EnvOverrideHost); h != "" {
			c.Host = h
			client.WithHostFromEnv()
		}
	}
}

func WithProxy(proxy ...string) Option {
	return func(config *Config) {
		config.Proxy = utils.StringArrayFilterEmpty(proxy)
	}
}
