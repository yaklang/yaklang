package thirdparty_bin

import "context"

type InstallOptionFunc func(o *InstallOptions)

func WithProgress(progress ProgressCallback) InstallOptionFunc {
	return func(o *InstallOptions) {
		o.Progress = progress
	}
}

func WithProxy(proxy string) InstallOptionFunc {
	return func(o *InstallOptions) {
		o.Proxy = proxy
	}
}

func WithForce(force bool) InstallOptionFunc {
	return func(o *InstallOptions) {
		o.Force = force
	}
}

func WithContext(ctx context.Context) InstallOptionFunc {
	return func(o *InstallOptions) {
		o.Context = ctx
	}
}

func _install(name string, options ...InstallOptionFunc) error {
	opts := &InstallOptions{}
	for _, option := range options {
		option(opts)
	}
	return Install(name, opts)
}

var Exports = map[string]interface{}{
	"Install":   _install,
	"Uninstall": Uninstall,
	"List":      GetAllStatus,

	"proxy":    WithProxy,
	"force":    WithForce,
	"context":  WithContext,
	"progress": WithProgress,
}
