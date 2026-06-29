package yakscripttools

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

const embedToolRoot = "yakscriptforai"

// ToolContentPreparer mutates yakscriptforai tool source before metadata parsing or execution.
type ToolContentPreparer func(namePath, content string) string

var toolContentPreparers []ToolContentPreparer

// RegisterToolContentPreparer registers a tool-pack-specific content preparer.
func RegisterToolContentPreparer(preparer ToolContentPreparer) {
	if preparer == nil {
		return
	}
	toolContentPreparers = append(toolContentPreparers, preparer)
}

// PrepareToolContent runs all registered preparers for the given embed tool path.
func PrepareToolContent(namePath, content string) string {
	namePath = normalizeEmbedToolPath(namePath)
	for _, preparer := range toolContentPreparers {
		content = preparer(namePath, content)
	}
	return content
}

// normalizeEmbedToolPath converts logical embed/tool paths to slash-separated form.
// Embed FS and AI tool paths are not OS filesystem paths; on Windows callers may
// still pass backslash paths via filepath.Join.
func normalizeEmbedToolPath(p string) string {
	p = strings.ReplaceAll(p, `\`, `/`)
	p = strings.Trim(p, `/`)
	p = path.Clean(p)
	if p == "." {
		return ""
	}
	return p
}

func isEmbedToolPathUnderPrefix(namePath, prefix string) bool {
	namePath = normalizeEmbedToolPath(namePath)
	prefix = normalizeEmbedToolPath(prefix)
	if namePath == prefix {
		return true
	}
	return strings.HasPrefix(namePath, prefix+"/")
}

// LibBundlePreparerConfig describes a yakscriptforai tool pack whose entry scripts
// share a lib/ directory concatenated at load/execution time.
type LibBundlePreparerConfig struct {
	ToolPrefix string
	LibDir     string
}

func (c LibBundlePreparerConfig) bundleMarker() string {
	return fmt.Sprintf("// __yaklang_tool_lib_bundle:%s__", normalizeEmbedToolPath(c.ToolPrefix))
}

func (c LibBundlePreparerConfig) libToolPath() string {
	libDir := normalizeEmbedToolPath(c.LibDir)
	root := normalizeEmbedToolPath(embedToolRoot)
	if rel, ok := strings.CutPrefix(libDir, root+"/"); ok {
		return rel
	}
	return libDir
}

func (c LibBundlePreparerConfig) isEntryTool(namePath string) bool {
	prefix := normalizeEmbedToolPath(c.ToolPrefix)
	namePath = normalizeEmbedToolPath(namePath)
	if !isEmbedToolPathUnderPrefix(namePath, prefix) {
		return false
	}
	libPath := c.libToolPath()
	if namePath == libPath || strings.HasPrefix(namePath, libPath+"/") {
		return false
	}
	return true
}

func (c LibBundlePreparerConfig) hasBundle(content string) bool {
	return strings.Contains(content, c.bundleMarker())
}

func (c LibBundlePreparerConfig) prepare(namePath, content string) string {
	if !c.isEntryTool(namePath) {
		return content
	}
	if c.hasBundle(content) {
		return content
	}
	libContent, err := loadLibContentFromEmbed(c.LibDir)
	if err != nil || libContent == "" {
		return content
	}
	return c.bundleMarker() + "\n" + libContent + "\n" + content
}

var (
	discoveredLibBundles     []LibBundlePreparerConfig
	discoveredLibBundlesOnce sync.Once
)

func init() {
	RegisterToolContentPreparer(prepareDiscoveredLibBundles)
}

func getDiscoveredLibBundles() []LibBundlePreparerConfig {
	discoveredLibBundlesOnce.Do(func() {
		if yakScriptFS == nil {
			InitEmbedFS()
		}
		discoveredLibBundles = discoverLibBundleConfigs()
	})
	return discoveredLibBundles
}

func discoverLibBundleConfigs() []LibBundlePreparerConfig {
	if yakScriptFS == nil {
		return nil
	}
	configs := []LibBundlePreparerConfig{}
	seen := map[string]struct{}{}
	_ = filesys.Recursive(embedToolRoot, filesys.WithFileSystem(yakScriptFS), filesys.WithDirStat(func(dirPath string, info fs.FileInfo) error {
		if info.Name() != "lib" {
			return nil
		}
		libDir := normalizeEmbedToolPath(dirPath)
		if _, ok := seen[libDir]; ok {
			return nil
		}
		if !libDirHasYakScripts(libDir) {
			return nil
		}
		root := normalizeEmbedToolPath(embedToolRoot)
		rel, ok := strings.CutPrefix(libDir, root+"/")
		if !ok || path.Base(libDir) != "lib" {
			return nil
		}
		toolPrefix := path.Dir(rel)
		if toolPrefix == "" || toolPrefix == "." {
			return nil
		}
		seen[libDir] = struct{}{}
		configs = append(configs, LibBundlePreparerConfig{
			ToolPrefix: toolPrefix,
			LibDir:     libDir,
		})
		return nil
	}))
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].ToolPrefix < configs[j].ToolPrefix
	})
	return configs
}

func matchLibBundleConfig(namePath string) *LibBundlePreparerConfig {
	var match *LibBundlePreparerConfig
	matchLen := -1
	for i := range getDiscoveredLibBundles() {
		cfg := &discoveredLibBundles[i]
		if !cfg.isEntryTool(namePath) {
			continue
		}
		if len(cfg.ToolPrefix) > matchLen {
			match = cfg
			matchLen = len(cfg.ToolPrefix)
		}
	}
	return match
}

func prepareDiscoveredLibBundles(namePath, content string) string {
	cfg := matchLibBundleConfig(namePath)
	if cfg == nil {
		return content
	}
	return cfg.prepare(namePath, content)
}

func libDirHasYakScripts(libDir string) bool {
	libDir = normalizeEmbedToolPath(libDir)
	entries, err := fs.ReadDir(yakScriptFS, libDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yak") {
			return true
		}
	}
	return false
}

func loadLibContentFromEmbed(libDir string) (string, error) {
	efs := yakScriptFS
	libDir = normalizeEmbedToolPath(libDir)
	entries, err := fs.ReadDir(efs, libDir)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yak") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		data, err := efs.ReadFile(path.Join(libDir, name))
		if err != nil {
			continue
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func NeedsLibBundlePrep(cfg LibBundlePreparerConfig, namePath, content string) bool {
	if !cfg.isEntryTool(namePath) {
		return false
	}
	return !cfg.hasBundle(content)
}

func NeedsLibBundlePrepForPath(namePath, content string) bool {
	namePath = normalizeEmbedToolPath(namePath)
	cfg := matchLibBundleConfig(namePath)
	if cfg == nil {
		return false
	}
	return !cfg.hasBundle(content)
}
