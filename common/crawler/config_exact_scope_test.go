package crawler

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDomainWhiteListExactPatternDoesNotImplicitlyExpand(t *testing.T) {
	crawler, err := NewCrawler(
		"https://seed.invalid/",
		WithExactOrigins(),
		WithDomainWhiteListExactPattern("Foo.Example"),
		WithDomainWhiteListExactPattern("127.0.0.1"),
	)
	require.NoError(t, err)

	for _, raw := range []string{
		"https://foo.example/inside",
		"https://FOO.EXAMPLE./case-and-fqdn",
		"http://127.0.0.1/inside",
	} {
		require.Truef(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, raw)), "expected exact scope to allow %s", raw)
	}
	for _, raw := range []string{
		"https://x.foo.example/implicit-subdomain",
		"https://foo.example.evil/implicit-suffix",
		"http://127.0.0.2/implicit-loopback-sibling",
	} {
		require.Falsef(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, raw)), "exact scope unexpectedly allowed %s", raw)
	}
}

func TestExactOriginSeedMatcherNormalizesCaseAndTrailingDot(t *testing.T) {
	crawler, err := NewCrawler("https://SEED.Example./", WithExactOrigins())
	require.NoError(t, err)

	for _, raw := range []string{
		"https://seed.example/inside",
		"https://SEED.EXAMPLE./case-and-fqdn",
	} {
		require.Truef(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, raw)), "expected normalized seed scope to allow %s", raw)
	}
	require.False(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, "https://x.seed.example/implicit-subdomain")))
	require.False(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, "https://seed.example.evil/implicit-suffix")))
}

func TestDomainWhiteListExactPatternHonorsOnlyExplicitGlob(t *testing.T) {
	crawler, err := NewCrawler(
		"https://seed.invalid/",
		WithExactOrigins(),
		WithDomainWhiteListExactPattern("*.redhaze.top"),
	)
	require.NoError(t, err)

	require.True(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, "https://coldchain.redhaze.top/app")))
	require.True(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, "https://COLDCHAIN-PROVIDER.REDHAZE.TOP/app")))
	require.False(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, "https://redhaze.top/")), "subdomain glob must not silently include the apex")
	require.False(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, "https://coldchain.redhaze.top.evil/")))
}

func TestDomainWhiteListExactPatternInvalidGlobFailsClosed(t *testing.T) {
	crawler, err := NewCrawler(
		"https://seed.invalid/",
		WithExactOrigins(),
		WithDomainWhiteListExactPattern("[invalid"),
	)
	require.NoError(t, err)
	require.False(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, "https://invalid/")))
}

func TestDomainBlackListNormalizesCaseAndFQDNTrailingDot(t *testing.T) {
	crawler, err := NewCrawler(
		"https://seed.invalid/",
		WithExactOrigins(),
		WithDomainWhiteListExactPattern("*.example.com"),
		WithDomainBlackList("Blocked.Example.COM."),
	)
	require.NoError(t, err)

	require.True(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, "https://allowed.example.com/inside")))
	for _, raw := range []string{
		"https://blocked.example.com/inside",
		"https://BLOCKED.EXAMPLE.COM./case-and-fqdn",
		"https://nested.blocked.example.com/legacy-suffix-match",
	} {
		require.Falsef(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, raw)), "normalized blacklist unexpectedly allowed %s", raw)
	}
}

func TestDomainBlackListWildcardPatternIsNotCompiledAsEmpty(t *testing.T) {
	crawler, err := NewCrawler(
		"https://seed.invalid/",
		WithExactOrigins(),
		WithDomainWhiteListExactPattern("*.example.com"),
		WithDomainBlackList("*.Blocked.Example.COM."),
	)
	require.NoError(t, err)

	require.True(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, "https://blocked.example.com/apex-not-covered-by-explicit-subdomain-glob")))
	for _, raw := range []string{
		"https://one.blocked.example.com/inside",
		"https://TWO.BLOCKED.EXAMPLE.COM./case-and-fqdn",
	} {
		require.Falsef(t, crawler.config.CheckShouldBeHandledURL(mustExactScopeURL(t, raw)), "wildcard blacklist unexpectedly allowed %s", raw)
	}
}

func mustExactScopeURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}
