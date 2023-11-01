package ssa4analyze

// config
type config struct {
}

func defaultConfig() config {
	return config{}
}

type Option func(*config)
