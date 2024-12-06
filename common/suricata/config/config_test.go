package config

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConfigFromYaml_CheckVarContain(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		varName  string
		s        any
		expected bool
	}{
		{
			"check single port", "HTTP_PORTS", 80, true,
		},
		{
			"check single port negative", "HTTP_PORTS", 90, false,
		},
		{
			"check shellcode port", "SHELLCODE_PORTS", 90, true,
		},
		{
			"check shellcode port negative", "SHELLCODE_PORTS", 80, false,
		},
		{
			"check port list", "FILE_DATA_PORTS", 143, true,
		},
		{
			"check port list use var", "FILE_DATA_PORTS", 80, true,
		},
		{
			"check port list negative", "FILE_DATA_PORTS", 111, false,
		},
		{
			"check ip list", "HOME_NET", "192.168.0.1", true,
		},
		{
			"check ip list negative", "HOME_NET", "123.123.123.123", false,
		},
		{
			"check bang apply to the ip list", "EXTERNAL_NET", "123.123.123.123", true,
		},
		{
			"check bang apply to the ip list negative", "EXTERNAL_NET", "192.168.0.1", false,
		},
		{
			"check ip var ref", "AIM_SERVERS", "123.123.123.123", true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.expected, DefaultConfig.MatchVar(testCase.varName, testCase.s))
		})
	}
}
func TestConfigFromCustom_CheckVarContain(t *testing.T) {
	config := NewConfig()
	for _, testCase := range []struct {
		name       string
		varName    string
		varValue   any
		checkVault any
		expected   bool
	}{
		{
			name:       "check single port",
			varName:    "HTTP_PORTS",
			varValue:   80,
			checkVault: 80,
			expected:   true,
		},
		{
			name:       "check single port negative 1",
			varName:    "SHELLCODE_PORTS",
			varValue:   "!80",
			checkVault: 80,
			expected:   false,
		},
		{
			name:       "check single port negative 1",
			varName:    "SHELLCODE_PORTS",
			varValue:   "!80",
			checkVault: 90,
			expected:   true,
		},
		{
			name:       "check single port negative 1",
			varName:    "SHELLCODE_PORTS",
			varValue:   "!$HTTP_PORTS",
			checkVault: 90,
			expected:   true,
		},
		{
			name:       "check ip list",
			varName:    "HOME_NET",
			varValue:   "[192.168.1.1]",
			checkVault: "192.168.1.1",
			expected:   true,
		},
		{
			name:       "check ip list 1",
			varName:    "HOME_NET",
			varValue:   "[192.168.1.1/16]",
			checkVault: "192.168.2.1",
			expected:   true,
		},
		{
			name:       "check ip list negative",
			varName:    "HOME_NET",
			varValue:   "[192.168.1.1]",
			checkVault: "192.168.1.2",
			expected:   false,
		},
		{
			name:       "check ip list with bang",
			varName:    "HOME_NET",
			varValue:   "![192.168.1.1]",
			checkVault: "192.168.1.2",
			expected:   true,
		},
		{
			name:       "check any",
			varName:    "HOME_NET",
			varValue:   "any",
			checkVault: "192.168.1.2",
			expected:   true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			config.AddVar(testCase.varName, testCase.varValue)
			assert.Equal(t, testCase.expected, config.MatchVar(testCase.varName, testCase.checkVault))
		})
	}
}
func TestConfigFromCustom_RandVar(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		varName  string
		varValue any
	}{
		{
			name:     "check ip list",
			varName:  "HOME_NET",
			varValue: "[192.168.1.1]",
		},
		{
			name:     "check ip list 1",
			varName:  "HOME_NET",
			varValue: "[192.168.1.1/16]",
		},
		{
			name:     "check ip list negative",
			varName:  "HOME_NET",
			varValue: "[192.168.1.1]",
		},
		{
			name:     "check ip list with bang",
			varName:  "HOME_NET",
			varValue: "![192.168.1.1]",
		},
		{
			name:     "check any",
			varName:  "HOME_NET",
			varValue: "any",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			config := NewConfig()
			config.AddVar(testCase.varName, testCase.varValue)
			ip := config.RandIpVar(testCase.varName)
			assert.Equal(t, true, config.MatchVar(testCase.varName, ip))
		})
	}
}
