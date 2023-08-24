package yakgit

import "context"

type config struct {
	Proxy   string
	Context context.Context
	Cancel  context.CancelFunc

	VerifyTLS          bool
	Username           string
	Password           string
	Depth              int
	RecursiveSubmodule bool
}

type Option func(*config) error

func WithProxy(proxy string) Option {
	return func(c *config) error {
		c.Proxy = proxy
		return nil
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *config) error {
		c.Context, c.Cancel = context.WithCancel(ctx)
		return nil
	}
}

func WithUsernamePassword(username, password string) Option {
	return func(c *config) error {
		c.Username = username
		c.Password = password
		return nil
	}
}
