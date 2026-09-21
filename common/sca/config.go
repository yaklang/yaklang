package sca

// SCAConfig retains filesystem-only configuration. Input acquisition belongs to the caller.
type SCAConfig struct {
	FileSystemPath   string
	DisableLanguages []string
}
type SCAConfigOption func(*SCAConfig)

func WithFileSystemPath(path string) SCAConfigOption {
	return func(c *SCAConfig) { c.FileSystemPath = path }
}
func WithDisableLanguages(languages ...string) SCAConfigOption {
	return func(c *SCAConfig) { c.DisableLanguages = languages }
}
