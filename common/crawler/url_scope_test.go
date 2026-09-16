package crawler

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestURLScopeOriginPortAndPath(t *testing.T) {
	option, err := WithURLScope("http://example.test:18080/cms", "https://example.test/allowed")
	require.NoError(t, err)
	crawler, err := NewCrawler("http://example.test:18080/cms", WithExactOrigins(), option)
	require.NoError(t, err)
	for _, raw := range []string{"http://example.test:18080/cms", "http://EXAMPLE.TEST.:18080/cms/a?x=1", "https://example.test:443/allowed/child"} {
		require.True(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, raw)), raw)
	}
	for _, raw := range []string{"http://example.test:18081/cms", "https://example.test:18080/cms", "http://example.test:18080/cms-other", "http://example.test:18080/cms/../outside", "http://example.test:18080/cms/%2e%2e/outside", "https://example.test/not-allowed"} {
		require.False(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, raw)), raw)
	}
	for _, raw := range []string{"", "/cms", "ftp://example.test", "https://user:pass@example.test", "https://example.test/?x=1", "https://example.test/#fragment"} {
		_, err := WithURLScope(raw)
		require.Error(t, err, raw)
	}
}
