package java_decompiler

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func aifixTreeURL(jarPath, dir string) string {
	return fmt.Sprintf("javadec:///jar-aifix?jar=%s&dir=%s",
		url.QueryEscape(jarPath), url.QueryEscape(dir))
}

func rawJavaDecGet(t *testing.T, action *Action, rawURL string) []*ypb.YakURLResource {
	t.Helper()
	u, err := CreateUrlFromString(rawURL)
	require.NoError(t, err)
	resp, err := action.Get(&ypb.RequestYakURLParams{Url: u, Method: "GET"})
	require.NoError(t, err)
	return resp.Resources
}

func resourceMap(resources []*ypb.YakURLResource) map[string]*ypb.YakURLResource {
	out := make(map[string]*ypb.YakURLResource, len(resources))
	for _, res := range resources {
		out[res.ResourceName] = res
	}
	return out
}

func resourceNames(resources []*ypb.YakURLResource) []string {
	out := make([]string, 0, len(resources))
	for _, res := range resources {
		out = append(out, res.ResourceName)
	}
	sort.Strings(out)
	return out
}

func extraValues(res *ypb.YakURLResource, key string) []string {
	var out []string
	for _, kv := range res.Extra {
		if kv.Key == key {
			out = append(out, kv.Value)
		}
	}
	return out
}

func queryParam(u *ypb.YakURL, key string) string {
	if u == nil {
		return ""
	}
	for _, kv := range u.Query {
		if kv.Key == key {
			return kv.Value
		}
	}
	return ""
}

func decodeResourceContent(t *testing.T, res *ypb.YakURLResource) []byte {
	t.Helper()
	encoded := extraValue(res, "content")
	require.NotEmpty(t, encoded, "%s has no content", res.Path)
	data, err := hex.DecodeString(encoded)
	require.NoError(t, err)
	return data
}

// TestJavaDecompilerAifixFrontendFlow follows the exact Yakit request sequence:
// javadec:///jar-aifix for the tree, javadec:///class-aifix with innerClasses
// for opening a file, and javadec:///export for the download.
func TestJavaDecompilerAifixFrontendFlow(t *testing.T) {
	probe := readEncodingProbeClass(t)
	nestedJar := makeTestJarBytes(t, map[string][]byte{
		"EncodingProbe.class": probe,
	})
	jarPath := writeTestJar(t, "aifix-view.jar", map[string][]byte{
		"com/example/Outer.class":       probe,
		"com/example/Outer$Inner.class": probe,
		"com/example/notes.txt":         []byte("notes\n"),
		"META-INF/MANIFEST.MF":          []byte("Manifest-Version: 1.0\n"),
		"lib/dep.jar":                   nestedJar,
	})

	action := NewJavaDecompilerAction()
	t.Cleanup(action.ClearCache)

	root := rawJavaDecGet(t, action, aifixTreeURL(jarPath, "."))
	rootByName := resourceMap(root)
	for _, name := range []string{"com", "lib", "META-INF"} {
		res := rootByName[name]
		require.NotNil(t, res, "missing directory %q in tree", name)
		require.Equal(t, "dir", res.ResourceType)
		require.Equal(t, "java-directory", res.VerboseType)
		require.True(t, res.HaveChildrenNodes, "%s must stay expandable", name)
		require.Equal(t, "/jar-aifix", res.Url.Path)
		require.Equal(t, name, queryParam(res.Url, "dir"))
	}
	for _, res := range root {
		require.False(t, strings.HasSuffix(res.ResourceName, ".class"),
			"tree must not show raw class entries: %s", res.ResourceName)
	}
	require.Equal(t, resourceNames(root), resourceNames(listJavaDecDir(t, action, jarPath, ".")),
		"/jar and /jar-aifix must expose the same tree")

	example := rawJavaDecGet(t, action, aifixTreeURL(jarPath, "com/example"))
	exampleByName := resourceMap(example)
	outer := exampleByName["Outer.java"]
	require.NotNil(t, outer, "class entry must be shown as Outer.java")
	require.Equal(t, "java-source", outer.VerboseType)
	require.Equal(t, "/class-aifix", outer.Url.Path)
	require.Equal(t, "com/example/Outer.java", queryParam(outer.Url, "class"))
	require.Equal(t, "com/example/Outer.class", extraValue(outer, "originalPath"))
	innerPaths := extraValues(outer, "innerClass")
	require.Equal(t, []string{"com/example/Outer$Inner.java"}, innerPaths)

	inner := exampleByName["Outer$Inner.java"]
	require.NotNil(t, inner, "inner class must stay addressable for class-aifix")
	require.Equal(t, "true", extraValue(inner, "hide"))
	require.Equal(t, "com/example/Outer.java", extraValue(inner, "outerClass"))
	require.Equal(t, "com/example/Outer$Inner.class", extraValue(inner, "originalPath"))

	notes := exampleByName["notes.txt"]
	require.NotNil(t, notes)
	require.Equal(t, "java-file", notes.VerboseType)

	// Frontend opens the file with the .java path plus innerClasses extras.
	classURL := fmt.Sprintf("javadec:///class-aifix?class=%s&jar=%s&innerClasses=%s",
		url.QueryEscape(queryParam(outer.Url, "class")),
		url.QueryEscape(jarPath),
		url.QueryEscape(strings.Join(innerPaths, ",")),
	)
	openResources := rawJavaDecGet(t, action, classURL)
	require.Len(t, openResources, 1)
	opened := openResources[0]
	require.Equal(t, "class", opened.ResourceType)
	require.Equal(t, "ai-enhanced-java-class", opened.VerboseType)
	require.Equal(t, "com/example/Outer.java", opened.Path)
	require.Contains(t, extraValue(opened, "innerClassesIncluded"), "Outer$Inner.java")
	openedText := assertJavaSource(t, opened.Path, decodeResourceContent(t, opened))
	require.Contains(t, openedText, "class ")

	// Legacy callers still send the raw .class name.
	legacyURL := fmt.Sprintf("javadec:///class-aifix?class=%s&jar=%s&innerClasses=%s",
		url.QueryEscape("com/example/Outer.class"),
		url.QueryEscape(jarPath),
		url.QueryEscape("com/example/Outer$Inner.class"),
	)
	legacyResources := rawJavaDecGet(t, action, legacyURL)
	require.Len(t, legacyResources, 1)
	assertJavaSource(t, "com/example/Outer.class", decodeResourceContent(t, legacyResources[0]))

	nestedEntries := rawJavaDecGet(t, action, aifixTreeURL(jarPath, "lib/dep.jar"))
	require.Len(t, nestedEntries, 1)
	require.Equal(t, "EncodingProbe.java", nestedEntries[0].ResourceName)
	require.Equal(t, "lib/dep.jar/EncodingProbe.class", extraValue(nestedEntries[0], "originalPath"))

	contents, names := exportJavaDecZip(t, action, jarPath)
	require.Contains(t, contents, "com/example/Outer.java")
	require.Contains(t, contents, "com/example/Outer$Inner.java")
	require.Contains(t, contents, "lib/dep.jar/EncodingProbe.java")
	for _, name := range names {
		require.False(t, strings.HasSuffix(strings.TrimSuffix(name, "/"), ".class"),
			"export must not contain raw class entries: %v", names)
	}
}
