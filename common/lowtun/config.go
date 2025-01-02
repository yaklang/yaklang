package lowtun

import (
	"errors"
	"runtime"
)

type Config struct {
	Name       string
	Driver     DriverProvider
	DeviceType DriverType
}

type Option func(config *Config)

func NewConfig(opts ...Option) (*Config, error) {
	config := &Config{}
	if runtime.GOOS == "darwin" {
		config.Driver = Driver_MacOSDriverSystem
		config.DeviceType = TUN
		config.Name = "utun111"
	} else {
		return nil, errors.New("unsupported platform")
	}
	for _, opt := range opts {
		opt(config)
	}
	return config, nil
}

func WithName(n string) Option {
	return func(config *Config) {
		config.Name = n
	}
}

func WithDriver(d DriverProvider) Option {
	return func(config *Config) {
		config.Driver = d
	}
}

func WithDeviceType(t DriverType) Option {
	return func(config *Config) {
		config.DeviceType = t
	}
}

