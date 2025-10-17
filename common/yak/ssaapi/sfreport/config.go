package sfreport

type Config struct {
	showFileContent  bool
	showDataflowPath bool
}

type Option func(*Config)

func WithFileContent(show bool) func(*Config) {
	return func(c *Config) {
		c.showFileContent = show
	}
}

func WithDataflowPath(show bool) func(*Config) {
	return func(c *Config) {
		c.showDataflowPath = show
	}
}
