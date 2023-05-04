package yakit

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestExtractTokenFromUrl(t *testing.T) {
	assert.True(t, ExtractTokenFromUrl("http://baidu.com/abc") == "abc")
	assert.True(t, ExtractTokenFromUrl("https://baidu.com/abc") == "abc")
	assert.True(t, ExtractTokenFromUrl("rmi://baidu.com/abc") == "abc")
	assert.True(t, ExtractTokenFromUrl("asdfas://baidu.com/abc") == "abc")
	assert.True(t, ExtractTokenFromUrl("goph+ar://baidu.com/abc") == "abc")
	assert.False(t, ExtractTokenFromUrl("goph+ar://baidu.com/abc") == "aaabc")
}
