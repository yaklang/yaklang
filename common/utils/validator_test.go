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
	assert.Equal(t, true, IsValidDomain("portal_zp_e.pdhr.com"))
	assert.Equal(t, true, IsValidDomain("_sip._tcp.example.com"))
	assert.Equal(t, false, IsValidDomain("-portal_zp_e.pdhr.com"))
	assert.Equal(t, false, IsValidDomain("portal_zp_e..pdhr.com"))
}

func TestIsValidHost(t *testing.T) {
	assert.Equal(t, true, IsValidHost("example.com"))
	assert.Equal(t, true, IsValidHost("example.com:80"))
	assert.Equal(t, true, IsValidHost("portal_zp_e.pdhr.com"))
	assert.Equal(t, true, IsValidHost("portal_zp_e.pdhr.com:8080"))
	assert.Equal(t, true, IsValidHost("127.0.0.1:443"))
	assert.Equal(t, true, IsValidHost("[::1]:80"))
	assert.Equal(t, false, IsValidHost("https://example.com"))
	assert.Equal(t, false, IsValidHost("https://example.com:443"))
	assert.Equal(t, false, IsValidHost("invalid target!@#"))
}
