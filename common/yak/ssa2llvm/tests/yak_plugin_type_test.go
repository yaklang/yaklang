package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

func TestYakPluginTypeNativeCLIArgs(t *testing.T) {
	code := `
name = cli.String("name", cli.setRequired(true))
count = cli.Int("count", cli.setDefault(1))
enabled = cli.Bool("enabled")
cli.check()
println(name)
if enabled {
	println(count + 40)
}
`

	output := runBinaryWithEnv(t, code, "", nil,
		withCompilePluginType(compiler.YakPluginTypeYak),
		withArgs("--name", "yaklang", "--count", "2", "--enabled"),
	)
	require.Contains(t, output, "yaklang\n")
	require.Contains(t, output, "42\n")
}

func TestYakPluginTypeNativeCLIFlagDefaultsFalse(t *testing.T) {
	code := `
enabled = cli.Bool("enabled")
cli.check()
if enabled {
	println("enabled")
} else {
	println("disabled")
}
`

	output := runBinaryWithEnv(t, code, "", nil,
		withCompilePluginType(compiler.YakPluginTypeYak),
	)
	require.Contains(t, output, "disabled\n")
	require.NotContains(t, output, "enabled\n")
}

func TestYakPluginTypeCodecWrapper(t *testing.T) {
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
}

func TestYakPluginTypePortScanWrapper(t *testing.T) {
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
}
