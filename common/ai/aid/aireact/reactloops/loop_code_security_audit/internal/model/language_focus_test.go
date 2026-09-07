package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectLanguageProfile(t *testing.T) {
	require.Equal(t, ProfileMemoryNative, DetectLanguageProfile("C/C++, CMake, Redis server"))
	require.Equal(t, ProfileManagedNetwork, DetectLanguageProfile("Java, Spring Boot, Maven"))
	require.Equal(t, ProfileManagedNetwork, DetectLanguageProfile("PHP Laravel"))
	require.Equal(t, ProfileSystemsGo, DetectLanguageProfile("Go, gin, etcd"))
	require.Equal(t, ProfileMixed, DetectLanguageProfile("Go + C FFI, Java admin UI"))
}

func TestOrderCategoriesByFocus_CPutsMemoryFirst(t *testing.T) {
	ordered := OrderCategoriesByFocus("C/C++ Redis", DefaultVulnCategories)
	require.NotEmpty(t, ordered)
	require.Equal(t, "memory_safety", ordered[0].ID)
}

func TestOrderCategoriesByFocus_JavaPutsNetworkFirst(t *testing.T) {
	ordered := OrderCategoriesByFocus("Java Spring Boot", DefaultVulnCategories)
	require.NotEmpty(t, ordered)
	// first should be from primary network set, not memory_safety
	require.NotEqual(t, "memory_safety", ordered[0].ID)
	require.Contains(t, []string{
		"sql_injection", "auth_bypass", "xxe_ssrf", "deserialization",
		"expression_injection", "xss_injection", "cmd_injection", "path_traversal", "code_execution",
	}, ordered[0].ID)
}

func TestLanguageFocusForCategory_TierText(t *testing.T) {
	msg := LanguageFocusForCategory("C/C++", "memory_safety")
	require.Contains(t, msg, "主攻")
	msg2 := LanguageFocusForCategory("Java Spring", "memory_safety")
	require.Contains(t, msg2, "低优先")
}
