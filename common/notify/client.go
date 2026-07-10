package notify

import (
	"context"
	"fmt"
	"sync"
)

type DriverConfig struct {
	SendConfig *SendConfig
	Options    Options
}

type Driver interface {
	Do(ctx context.Context, req *Request) (*Response, error)
	Stream(ctx context.Context, req *Request, emit EventHandler) error
}

type Descriptor struct {
	Platform     Platform
	Capabilities Capabilities
	Actions      []Action
	New          func(DriverConfig) (Driver, error)
}

type Registry struct {
	mu          sync.RWMutex
	descriptors map[Platform]Descriptor
}

func NewRegistry() *Registry {
	return &Registry{descriptors: map[Platform]Descriptor{}}
}

func (r *Registry) Register(desc Descriptor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.descriptors[desc.Platform] = desc
}

func (r *Registry) Descriptor(platform Platform) (Descriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.descriptors[platform]
	return desc, ok
}

func (r *Registry) Supports(platform Platform, action Action) bool {
	desc, ok := r.Descriptor(platform)
	if !ok {
		return false
	}
	for _, item := range desc.Actions {
		if item == action {
			return true
		}
	}
	return false
}

type Client struct {
	registry   *Registry
	sendConfig *SendConfig
}

type ClientOption func(*Client)

func WithRegistry(reg *Registry) ClientOption {
	return func(c *Client) {
		if reg != nil {
			c.registry = reg
		}
	}
}

func WithSendConfig(cfg *SendConfig) ClientOption {
	return func(c *Client) {
		c.sendConfig = cfg
	}
}

func NewClient(opts ...ClientOption) *Client {
	c := &Client{registry: NewRegistry()}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Do(ctx context.Context, req *Request) (*Response, error) {
	req, desc, err := c.prepare(req)
	if err != nil {
		return nil, err
	}
	d, err := desc.New(DriverConfig{SendConfig: c.sendConfig, Options: req.Options})
	if err != nil {
		return nil, fmt.Errorf("notify: create driver %s: %w", req.Platform, err)
	}
	return d.Do(ctx, req)
}

func (c *Client) Stream(ctx context.Context, req *Request, emit EventHandler) error {
	req, desc, err := c.prepare(req)
	if err != nil {
		return err
	}
	d, err := desc.New(DriverConfig{SendConfig: c.sendConfig, Options: req.Options})
	if err != nil {
		return fmt.Errorf("notify: create driver %s: %w", req.Platform, err)
	}
	return d.Stream(ctx, req, emit)
}

func (c *Client) prepare(req *Request) (*Request, Descriptor, error) {
	if req == nil {
		return nil, Descriptor{}, fmt.Errorf("notify: nil request")
	}
	if req.URL != "" {
		parsed, err := ParseURL(req.URL)
		if err != nil {
			return nil, Descriptor{}, err
		}
		parsed.Credential = req.Credential
		parsed.Target = req.Target
		parsed.Message = req.Message
		parsed.Resource = req.Resource
		parsed.Options = req.Options
		parsed.Native = req.Native
		req = parsed
	}
	desc, ok := c.registry.Descriptor(req.Platform)
	if !ok {
		return nil, Descriptor{}, fmt.Errorf("notify: unknown platform %q", req.Platform)
	}
	if !c.registry.Supports(req.Platform, req.Action) {
		return nil, Descriptor{}, fmt.Errorf("notify: platform %q does not support action %q", req.Platform, req.Action)
	}
	return req, desc, nil
}
