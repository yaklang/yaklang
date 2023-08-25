package yakgit

import (
	"context"
	"github.com/go-git/go-git/v5"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

type config struct {
	Context context.Context
	Cancel  context.CancelFunc

	VerifyTLS          bool
	Username           string
	Password           string
	Depth              int
	RecursiveSubmodule bool

	// remote operation
	Remote string

	// Force
	Force        bool
	NoFetchTags  bool
	FetchAllTags bool

	CheckoutCreate bool
	CheckoutForce  bool
	CheckoutKeep   bool
}

func NewConfig() *config {
	ctx, cancel := context.WithCancel(context.Background())
	return &config{
		Context:            ctx,
		Cancel:             cancel,
		VerifyTLS:          true,
		Depth:              1,
		RecursiveSubmodule: true,
		Remote:             "origin",
	}
}

func (c *config) ToRecursiveSubmodule() git.SubmoduleRescursivity {
	var recursiveSubmodule git.SubmoduleRescursivity
	if c.RecursiveSubmodule {
		recursiveSubmodule = git.SubmoduleRescursivity(10)
	} else {
		recursiveSubmodule = git.NoRecurseSubmodules
	}
	return recursiveSubmodule
}

func (c *config) ToAuth() gitHttp.AuthMethod {
	var auth gitHttp.AuthMethod
	if c.Username != "" && c.Password != "" {
		auth = &gitHttp.BasicAuth{
			Username: c.Username,
			Password: c.Password,
		}
	}
	return auth
}

type Option func(*config) error

func WithVerifyTLS(b bool) Option {
	return func(c *config) error {
		c.VerifyTLS = b
		return nil
	}
}

func WithNoFetchTags(b bool) Option {
	return func(c *config) error {
		c.NoFetchTags = b
		return nil
	}
}

func WithFetchAllTags(b bool) Option {
	return func(c *config) error {
		c.FetchAllTags = b
		return nil
	}
}

func WithCheckoutCreate(b bool) Option {
	return func(c *config) error {
		c.CheckoutCreate = b
		return nil
	}
}

func WithCheckoutForce(b bool) Option {
	return func(c *config) error {
		c.CheckoutForce = b
		return nil
	}
}

func WithCheckoutKeep(b bool) Option {
	return func(c *config) error {
		c.CheckoutKeep = b
		return nil
	}
}

func WithDepth(depth int) Option {
	return func(c *config) error {
		c.Depth = depth
		return nil
	}
}

func WithForce(b bool) Option {
	return func(c *config) error {
		c.Force = b
		return nil
	}
}

func WithRemote(remote string) Option {
	return func(c *config) error {
		c.Remote = remote
		return nil
	}
}

func WithRecuriveSubmodule(b bool) Option {
	return func(c *config) error {
		c.RecursiveSubmodule = b
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
