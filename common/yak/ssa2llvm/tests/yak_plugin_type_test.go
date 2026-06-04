package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

func TestYakPluginTypeNativeCLI(t *testing.T) {
	code := `
name = cli.String("name")
count = cli.Int("count", cli.setDefault(1))
enabled = cli.Bool("enabled")
cli.check()
if name != "" {
	println(name)
}
if enabled {
	println(count + 40)
} else if name == "" {
	println("disabled")
}
`

	cfg := newRunBinaryConfig(t, withCompilePluginType(compiler.YakPluginTypeYak))
	bin, cleanup := compileBinary(t, code, "", cfg)
	defer cleanup()

	t.Run("args", func(t *testing.T) {
		output := runCompiledBinaryWithEnv(t, bin, nil, "--name", "yaklang", "--count", "2", "--enabled")
		require.Contains(t, output, "yaklang\n")
		require.Contains(t, output, "42\n")
	})
	t.Run("bool_default_false", func(t *testing.T) {
		output := runCompiledBinaryWithEnv(t, bin, nil)
		require.Contains(t, output, "disabled\n")
		require.NotContains(t, output, "enabled\n")
	})
}

func TestYakPluginTypeWrappers(t *testing.T) {
	t.Run("codec", func(t *testing.T) {
		code := `
handle = func(param) {
	return codec.EncodeBase64(param)
}
`
		output := runBinaryWithEnv(t, code, "", nil,
			withCompilePluginType(compiler.YakPluginTypeCodec),
			withArgs("--param", "yaklang"),
		)
		require.Contains(t, strings.TrimSpace(output), "eWFrbGFuZw==")
	})
	t.Run("port_scan", func(t *testing.T) {
		code := `
handle = func(result) {
	println(result.Target)
	println(result.Port)
	println(result.Fingerprint.ServiceName)
}
`
		output := runBinaryWithEnv(t, code, "", nil,
			withCompilePluginType(compiler.YakPluginTypePortScan),
			withArgs("--target", "127.0.0.1", "--port", "3306", "--service", "mysql"),
		)
		require.Contains(t, output, "127.0.0.1\n")
		require.Contains(t, output, "3306\n")
		require.Contains(t, output, "mysql\n")
	})
}
