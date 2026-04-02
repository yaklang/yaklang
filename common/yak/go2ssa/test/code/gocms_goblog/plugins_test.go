package plugins

import (
	"embed"
	"fmt"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/yaegi/interp"
	"go.goblog.app/app/pkgs/plugintypes"
	"go.goblog.app/app/pkgs/yaegiwrappers"
)

//go:embed sample/*
var sampleSourceFS embed.FS

func TestLoadPluginEmbeddedSuccess(t *testing.T) {
	host := NewPluginHost(
		map[string]reflect.Type{
			"stringer": reflect.TypeFor[fmt.Stringer](),
		},
		interp.Exports{},
		sampleSourceFS,
	)

	_, err := host.LoadPlugin(&PluginConfig{
		Path:       "embedded:sample",
		ImportPath: "sample",
	})
	require.NoError(t, err)

	plugins := host.GetPlugins("stringer")
	require.Len(t, plugins, 1)
	assert.Equal(t, "ok", plugins[0].(fmt.Stringer).String())
}

func TestLoadPluginFailsOnMissingSource(t *testing.T) {
	host := NewPluginHost(map[string]reflect.Type{}, interp.Exports{}, fstest.MapFS{})
	_, err := host.LoadPlugin(&PluginConfig{
		Path:       "embedded:missing",
		ImportPath: "missing",
	})
	assert.Error(t, err)
}

func TestLoadPluginRegistersHooks(t *testing.T) {
	source := fstest.MapFS{
		"hooktest/src/hooktest/hooktest.go": {
			Data: []byte(`package hooktest

import "go.goblog.app/app/pkgs/plugintypes"

type plugin struct{}

func GetPlugin() (plugintypes.PostCreatedHook, plugintypes.PostUpdatedHook) {
	return plugin{}, plugin{}
}

func (plugin) PostCreated(_ plugintypes.Post) {}
func (plugin) PostUpdated(_ plugintypes.Post) {}
`),
		},
	}

	host := NewPluginHost(
		map[string]reflect.Type{
			"postcreated": reflect.TypeFor[plugintypes.PostCreatedHook](),
			"postupdated": reflect.TypeFor[plugintypes.PostUpdatedHook](),
		},
		yaegiwrappers.Symbols,
		source,
	)

	plugins, err := host.LoadPlugin(&PluginConfig{
		Path:       "embedded:hooktest",
		ImportPath: "hooktest",
	})
	require.NoError(t, err)
	assert.Contains(t, plugins, "postcreated")
	assert.Contains(t, plugins, "postupdated")

	assert.Len(t, host.GetPlugins("postcreated"), 1)
	assert.Len(t, host.GetPlugins("postupdated"), 1)
}
