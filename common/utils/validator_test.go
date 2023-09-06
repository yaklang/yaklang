package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestIsValidDomain(t *testing.T) {
	assert.Equal(t, true, IsValidDomain("www.baidu.com"))
	assert.Equal(t, true, IsValidDomain("www.baidu.com.cn"))
	assert.Equal(t, false, IsValidDomain("."))
	assert.Equal(t, true, IsValidDomain("abc.中国"))
	assert.Equal(t, true, IsValidDomain("xn--ccccc.cn"))
	assert.Equal(t, false, IsValidDomain("-aa.example.cc"))
	assert.Equal(t, true, IsValidDomain("localhost"))
}
