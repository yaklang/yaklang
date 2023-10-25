package ssa4analyze

// config
type config struct {
	// for extern instance
	ExternValue *map[string]any
	ExternLib   *map[string]map[string]any
}

func defaultConfig() config {
	return config{}
}

type Option func(*config)

func WithExternLib(lib *map[string]map[string]any) Option {
	return func(c *config) {
		c.ExternLib = lib
	}
}

func WithExternValue(value *map[string]any) Option {
	return func(c *config) {
		c.ExternValue = value
	}
}
