package yakscripttools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireDiscoveredLibBundle(t *testing.T, toolPrefix string) LibBundlePreparerConfig {
	t.Helper()
	InitEmbedFS()
	for _, cfg := range discoverLibBundleConfigs() {
		if cfg.ToolPrefix == toolPrefix {
			return cfg
		}
	}
	t.Fatalf("lib bundle for %q not discovered under %s/*/lib/", toolPrefix, embedToolRoot)
	return LibBundlePreparerConfig{}
}

func TestDiscoverLibBundleConfigs_FindsJavaAudit(t *testing.T) {
	InitEmbedFS()
	cfg := requireDiscoveredLibBundle(t, "java_audit")
	require.Equal(t, "yakscriptforai/java_audit/lib", cfg.LibDir)
}

func TestLibBundlePreparer_PrependsLibOnWindows(t *testing.T) {
	InitEmbedFS()
	cfg := requireDiscoveredLibBundle(t, "java_audit")
	raw := `__DESC__ = "x"
target = cli.String("target", cli.setRequired(true))
cli.check()
report = javaAuditRunArchInfo(target, "dubbo", {})
javaAuditEmitReport(report, "")`
	prepared := cfg.prepare("java_audit/dubbo_arch_info", raw)
	require.Contains(t, prepared, cfg.bundleMarker())
	require.Contains(t, prepared, "javaAuditRunArchInfo =")
	require.Greater(t, strings.Count(prepared, "\n"), 100)
}

func TestLibBundlePreparer_Idempotent(t *testing.T) {
	InitEmbedFS()
	cfg := requireDiscoveredLibBundle(t, "java_audit")
	first := cfg.prepare("java_audit/dubbo_arch_info", `cli.check()`)
	second := cfg.prepare("java_audit/dubbo_arch_info", first)
	require.Equal(t, first, second)
}

func TestLibBundlePreparer_SkipsLibSourcePaths(t *testing.T) {
	cfg := requireDiscoveredLibBundle(t, "java_audit")
	raw := "__DESC__ = \"x\"\ncli.check()"
	require.False(t, cfg.isEntryTool("java_audit/lib/common"))
	require.Equal(t, raw, cfg.prepare("java_audit/lib/common", raw))
}

func TestLibBundlePreparer_DoesNotFalsePositiveOnLibSubstring(t *testing.T) {
	cfg := requireDiscoveredLibBundle(t, "java_audit")
	require.True(t, cfg.isEntryTool("java_audit/library_scan"))
}

func TestPrepareToolContent_AutoPrependsDiscoveredLib(t *testing.T) {
	InitEmbedFS()
	cfg := requireDiscoveredLibBundle(t, "java_audit")
	prepared := PrepareToolContent("java_audit/dubbo_arch_info", `cli.check()`)
	require.Contains(t, prepared, cfg.bundleMarker())
}

func TestNeedsLibBundlePrep(t *testing.T) {
	cfg := requireDiscoveredLibBundle(t, "java_audit")
	require.False(t, NeedsLibBundlePrep(cfg, "http/do_http_request", "cli.check()"))
	require.True(t, NeedsLibBundlePrep(cfg, "java_audit/dubbo_arch_info", "cli.check()"))
	require.False(t, NeedsLibBundlePrep(cfg, "java_audit/dubbo_arch_info", cfg.bundleMarker()+"\ncli.check()"))
	require.False(t, NeedsLibBundlePrep(cfg, "java_audit/lib/common", "x = 1"))
}

func TestPrepareToolContent_AcceptsWindowsStyleToolPath(t *testing.T) {
	InitEmbedFS()
	cfg := requireDiscoveredLibBundle(t, "java_audit")
	prepared := PrepareToolContent(`java_audit\dubbo_arch_info`, `cli.check()`)
	require.Contains(t, prepared, cfg.bundleMarker())
}

func TestNormalizeEmbedToolPath(t *testing.T) {
	require.Equal(t, "java_audit/dubbo_arch_info", normalizeEmbedToolPath(`java_audit\dubbo_arch_info`))
	require.Equal(t, "yakscriptforai/java_audit/lib", normalizeEmbedToolPath(`yakscriptforai\java_audit\lib\`))
}

func TestNeedsLibBundlePrepForPath(t *testing.T) {
	require.True(t, NeedsLibBundlePrepForPath("java_audit/dubbo_arch_info", "cli.check()"))
	require.False(t, NeedsLibBundlePrepForPath("java_audit/lib/common", "x = 1"))
	require.True(t, NeedsLibBundlePrepForPath(`java_audit\dubbo_arch_info`, "cli.check()"))
}
