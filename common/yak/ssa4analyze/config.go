package ssa4analyze

// config
type config struct {
	analyzers  []Analyzer
	enablePass bool // if true enable pass, analyzer more information
}

func defaultConfig() config {
	return config{
		analyzers:  analyzers,
		enablePass: true,
	}
}

type Option func(*config)

func WithAnalyzer(a []Analyzer) Option {
	return func(c *config) {
		c.analyzers = a
	}
}
func AddAnalyzer(a Analyzer) Option {
	return func(c *config) {
		c.analyzers = append(c.analyzers, a)
	}
}

func WithPass(b bool) Option {
	return func(c *config) {
		c.enablePass = b
	}
}
