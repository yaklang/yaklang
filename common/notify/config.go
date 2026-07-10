package notify

import "time"

type SendConfig struct {
	AppID             string
	AppSecret         string
	RobotSecret       string
	VerificationToken string
	EncryptKey        string
	Proxy             string
	BaseURL           string
	Timeout           time.Duration
}

type SendOption func(*SendConfig)

func NewSendConfig(opts ...SendOption) *SendConfig {
	c := &SendConfig{Timeout: 30 * time.Second}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

func WithAppID(id string) SendOption {
	return func(c *SendConfig) { c.AppID = id }
}

func WithAppSecret(secret string) SendOption {
	return func(c *SendConfig) { c.AppSecret = secret }
}

func WithRobotSecret(s string) SendOption {
	return func(c *SendConfig) { c.RobotSecret = s }
}

func WithVerificationToken(t string) SendOption {
	return func(c *SendConfig) { c.VerificationToken = t }
}

func WithEncryptKey(k string) SendOption {
	return func(c *SendConfig) { c.EncryptKey = k }
}

func WithProxy(p string) SendOption {
	return func(c *SendConfig) { c.Proxy = p }
}

func WithBaseURL(u string) SendOption {
	return func(c *SendConfig) { c.BaseURL = u }
}

func WithTimeout(d time.Duration) SendOption {
	return func(c *SendConfig) { c.Timeout = d }
}

func (c *SendConfig) AsSendOptions() []SendOption {
	if c == nil {
		return nil
	}
	opts := []SendOption{
		WithAppID(c.AppID),
		WithAppSecret(c.AppSecret),
		WithRobotSecret(c.RobotSecret),
		WithVerificationToken(c.VerificationToken),
		WithEncryptKey(c.EncryptKey),
		WithProxy(c.Proxy),
		WithBaseURL(c.BaseURL),
	}
	if c.Timeout > 0 {
		opts = append(opts, WithTimeout(c.Timeout))
	}
	return opts
}
