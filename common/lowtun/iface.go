package lowtun

import (
	"errors"
	"io"
)

type Interface struct {
	io.ReadWriteCloser

	Tap  bool
	name string
}

func New(opts ...Option) (*Interface, error) {
	config, err := NewConfig(opts...)
	if err != nil {
		return nil, err
	}
	switch config.DeviceType {
	case TUN, TAP:
		return openDev(config)
	}
	return nil, errors.New("unsupported device type")
}

func (i *Interface) IsTUN() bool {
	return !i.Tap
}

func (i *Interface) IsTAP() bool {
	return i.Tap
}

func (i *Interface) Name() string {
	return i.name
}
