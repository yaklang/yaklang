package loop_yaklangcode

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildSelfTestCLIArgs_RequiredTarget(t *testing.T) {
	code := `
TARGET_URL = cli.String("target", cli.setRequired(true), cli.setHelp("url"))
SQLI_ENABLE = cli.Bool("sqli-enable", cli.setDefault(true))
BRUTE_ENABLE = cli.Bool("brute-enable", cli.setDefault(false))
cli.check()
if YAK_MAIN { runSelfTest() }
`
	args := buildSelfTestCLIArgs(code)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "--target")
	assert.Contains(t, joined, "127.0.0.1")
	assert.Contains(t, joined, "--sqli-enable")
	assert.Contains(t, joined, "false")
}

func TestBuildSelfTestCLIArgs_SignKeyWithDefault(t *testing.T) {
	code := `
SIGN_KEY = cli.String("sign-key", cli.setDefault("your-secret-key"))
TARGET_URL = cli.String("target", cli.setRequired(true))
cli.check()
`
	args := buildSelfTestCLIArgs(code)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "--target")
	assert.NotContains(t, joined, "--sign-key")
}

func TestBuildSelfTestCLIArgs_NoCliCheckSkipsOptional(t *testing.T) {
	code := `
OPT = cli.String("opt", cli.setDefault("x"))
if YAK_MAIN { runSelfTest() }
`
	args := buildSelfTestCLIArgs(code)
	assert.Empty(t, args)
}

func TestFindMatchingParen(t *testing.T) {
	s := `cli.String("t", cli.setHelp("hello (world)"), cli.setRequired(true))`
	close := findMatchingParen(s, strings.Index(s, "("))
	assert.Greater(t, close, 0)
	assert.Equal(t, byte(')'), s[close])
}
