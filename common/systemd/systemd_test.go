package systemd

import (
	"testing"
)

func TestNewSystemServiceConfig(t *testing.T) {
	// bash -c 'yak -c aGlkcy5TZXRNb25pdG9ySW50ZXJ2YWwoMSkKY21kID0gYGJhc2ggLWMgJ2NkIC9yb290L3Z1bGlub25lLyAmJiB5YWsgbWFuYWdlLnlhayAtYyB1cCdgCmR1bXAoZXhlYy5TeXN0ZW0oY21kKSkKaGlkcy5DUFVBdmVyYWdlQ2FsbGJhY2socGVyY2VudCA9PiB7CiAgICBpZiBwZXJjZW50ID4gOTAgewogICAgICAgIGV4ZWMuU3lzdGVtKCJyZWJvb3QiKQogICAgfQp9KQpmb3IgewogICAgc2xlZXAoMSkKfQ== --base64'
	_, c := NewSystemServiceConfig(
		"Monitor Vulinbox",
		WithServiceExecStart(`bash -c 'yak -c aGlkcy5TZXRNb25pdG9ySW50ZXJ2YWwoMSkKY21kID0gYGJhc2ggLWMgJ2NkIC9yb290L3Z1bGlub25lLyAmJiB5YWsgbWFuYWdlLnlhayAtYyB1cCdgCmR1bXAoZXhlYy5TeXN0ZW0oY21kKSkKaGlkcy5DUFVBdmVyYWdlQ2FsbGJhY2socGVyY2VudCA9PiB7CiAgICBpZiBwZXJjZW50ID4gOTAgewogICAgICAgIGV4ZWMuU3lzdGVtKCJyZWJvb3QiKQogICAgfQp9KQpmb3IgewogICAgc2xlZXAoMSkKfQ== --base64'`),
	).ToServiceFile()
	if string(c) == "" {
		panic("empty")
	}
	println(string(c))
}
