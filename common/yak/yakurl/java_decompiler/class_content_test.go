package java_decompiler

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newJavaDecTestURL(t *testing.T, route string, query ...*ypb.KVPair) *ypb.RequestYakURLParams {
	t.Helper()
	u, err := CreateUrlFromString("javadec://" + route)
	require.NoError(t, err)
	u.Query = append(u.Query, query...)
	return &ypb.RequestYakURLParams{Url: u, Method: "GET"}
}

func createEncodingProbeJar(t *testing.T) string {
	t.Helper()
	return writeTestJar(t, "encoding-probe.jar", map[string][]byte{
		"EncodingProbe.class": readEncodingProbeClass(t),
	})
}

func readEncodingProbeClass(t *testing.T) []byte {
	t.Helper()
	fixture := filepath.Join("..", "..", "cmd", "yakcmds", "testdata", "java_encoding", "EncodingProbe.class")
	raw, err := os.ReadFile(fixture)
	require.NoError(t, err)
	return raw
}

func makeTestJarBytes(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func writeTestJar(t *testing.T, name string, entries map[string][]byte) string {
	t.Helper()
	jarPath := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(jarPath, makeTestJarBytes(t, entries), 0o644))
	return jarPath
}

func listJavaDecDir(t *testing.T, action *Action, jarPath, dir string) []*ypb.YakURLResource {
	t.Helper()
	resp, err := action.Get(newJavaDecTestURL(t, "/jar",
		&ypb.KVPair{Key: "jar", Value: jarPath},
		&ypb.KVPair{Key: "dir", Value: dir},
	))
	require.NoError(t, err)
	return resp.Resources
}

func readJavaDecClass(t *testing.T, action *Action, jarPath, classPath string) (*ypb.YakURLResource, []byte) {
	t.Helper()
	resp, err := action.Get(newJavaDecTestURL(t, "/class",
		&ypb.KVPair{Key: "jar", Value: jarPath},
		&ypb.KVPair{Key: "class", Value: classPath},
	))
	require.NoError(t, err)
	require.Len(t, resp.Resources, 1)
	resource := resp.Resources[0]

	var hexContent string
	for _, kv := range resource.Extra {
		if kv.Key == "content" {
			hexContent = kv.Value
			break
		}
	}
	require.NotEmpty(t, hexContent, "class resource must carry hex content")
	data, err := hex.DecodeString(hexContent)
	require.NoError(t, err)
	return resource, data
}

func assertJavaSource(t *testing.T, classPath string, data []byte) string {
	t.Helper()
	require.False(t, bytes.HasPrefix(data, []byte{0xca, 0xfe, 0xba, 0xbe}),
		"%s returned a raw class file (CAFEBABE)", classPath)
	require.True(t, utf8.Valid(data), "%s is not valid UTF-8", classPath)
	require.NotContains(t, string(data), "\x00", "%s contains binary NUL bytes", classPath)
	text := string(data)
	require.True(t, looksLikeJavaSource(text), "%s does not look like Java source", classPath)
	return text
}

func looksLikeJavaSource(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "// ") {
		return true
	}
	for _, marker := range []string{
		"class ", "interface ", "enum ", "record ", "@interface ",
		"module ", "open module ", "package ",
		"// original ", "// decompile ", "// javajive ",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func walkJavaDecClasses(t *testing.T, action *Action, jarPath string, limit int) []string {
	t.Helper()
	seenDir := map[string]bool{".": true}
	queue := []string{"."}
	var classes []string
	for len(queue) > 0 && (limit <= 0 || len(classes) < limit) {
		dir := queue[0]
		queue = queue[1:]
		for _, res := range listJavaDecDir(t, action, jarPath, dir) {
			if res.ResourceType == "dir" || javaclassparserIsArchiveLeaf(res.ResourceName) {
				if !seenDir[res.Path] {
					seenDir[res.Path] = true
					queue = append(queue, res.Path)
				}
				continue
			}
			require.False(t, strings.HasSuffix(strings.ToLower(res.ResourceName), ".class"),
				"listing must not expose raw class entries, got %q in %s", res.ResourceName, dir)
			if !strings.HasSuffix(res.ResourceName, ".java") {
				continue
			}
			classes = append(classes, res.Path)
			if limit > 0 && len(classes) >= limit {
				break
			}
		}
	}
	return classes
}

func extraValue(resource *ypb.YakURLResource, key string) string {
	for _, kv := range resource.Extra {
		if kv.Key == key {
			return kv.Value
		}
	}
	return ""
}

// randomJavaDecSample returns n entries with a fixed seed so CI failures stay
// reproducible.
func randomJavaDecSample(paths []string, n int) []string {
	if len(paths) <= n {
		return append([]string(nil), paths...)
	}
	r := rand.New(rand.NewSource(20261119))
	indexes := r.Perm(len(paths))
	out := make([]string, 0, n)
	for _, index := range indexes[:n] {
		out = append(out, paths[index])
	}
	return out
}

func exportJavaDecZip(t *testing.T, action *Action, jarPath string) (map[string][]byte, []string) {
	t.Helper()
	exportResp, err := action.Get(newJavaDecTestURL(t, "/export",
		&ypb.KVPair{Key: "jar", Value: jarPath},
	))
	require.NoError(t, err)
	require.Len(t, exportResp.Resources, 1)
	zipPath := extraValue(exportResp.Resources[0], "path")
	require.NotEmpty(t, zipPath)
	t.Logf("export zip: %s", zipPath)

	zr, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer zr.Close()

	contents := make(map[string][]byte)
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		contents[f.Name] = data
	}
	return contents, names
}

// assertJavaSourceKeywords rejects binary payloads and comment-only stubs that
// carry no Java declaration.
func javaSourceHasDeclaration(text string) bool {
	for _, marker := range []string{"class ", "interface ", "enum ", "record ", "package ", "module "} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func assertJavaSourceKeywords(t *testing.T, classPath, text string) {
	t.Helper()
	if javaSourceHasDeclaration(text) {
		return
	}
	// javajive renders some anonymous/member bodies as a comment-only placeholder.
	if strings.Contains(text, "body owned by") || strings.Contains(text, "decompile fallback") {
		return
	}
	preview := text
	if len(preview) > 240 {
		preview = preview[:240]
	}
	require.Failf(t, "missing Java declaration", "%s does not look like Java source: %q", classPath, preview)
}

// TestJavaDecompilerClassRouteReturnsSourceFromSmallJar is the fast regression
// test: a one-class jar must be listed as Java source and served as Java source.
func TestJavaDecompilerClassRouteReturnsSourceFromSmallJar(t *testing.T) {
	jarPath := createEncodingProbeJar(t)
	action := NewJavaDecompilerAction()
	t.Cleanup(action.ClearCache)

	entries := listJavaDecDir(t, action, jarPath, ".")
	require.Len(t, entries, 1)
	classEntry := entries[0]
	require.Equal(t, "EncodingProbe.java", classEntry.ResourceName)
	require.Equal(t, "EncodingProbe.java", classEntry.Path)
	require.Equal(t, "file", classEntry.ResourceType)
	require.Equal(t, "java-source", classEntry.VerboseType)
	require.Equal(t, "/class", classEntry.Url.Path)
	require.Equal(t, "EncodingProbe.class", extraValue(classEntry, "originalPath"))

	resource, data := readJavaDecClass(t, action, jarPath, classEntry.Path)
	require.Equal(t, "class", resource.ResourceType)
	require.Equal(t, "decompiled-java-class", resource.VerboseType)

	text := assertJavaSource(t, classEntry.Path, data)
	require.Contains(t, text, "中文字符串常量")
	require.Contains(t, text, "日本語の文字列")
	require.Contains(t, text, "한국어 문자열")
	require.Contains(t, text, "café naïve © ®")
	require.NotContains(t, text, "\uFFFD")
	assertJavaSourceKeywords(t, classEntry.Path, text)

	// Older clients still send the raw .class name.
	legacyResource, legacyData := readJavaDecClass(t, action, jarPath, "EncodingProbe.class")
	require.Equal(t, "EncodingProbe.class", legacyResource.Path)
	require.Equal(t, data, legacyData)

	contents, names := exportJavaDecZip(t, action, jarPath)
	t.Logf("export entries: %v", names)
	for _, name := range names {
		require.False(t, strings.HasSuffix(strings.TrimSuffix(name, "/"), ".class"),
			"export must not contain raw class entries: %v", names)
	}
	require.Contains(t, contents, "EncodingProbe.java")
	exported := contents["EncodingProbe.java"]
	assertJavaSource(t, "EncodingProbe.java", exported)
	require.Equal(t, data, exported)
}

// TestJavaDecompilerClassRouteRealJars samples the two reported jars. Set
// JAVA_DECOMPILER_TEST_JARS to a comma-separated list to enable it.
// Set JAVA_DECOMPILER_FULL=1 to verify and export every class in every jar.
func TestJavaDecompilerClassRouteRealJars(t *testing.T) {
	rawJars := os.Getenv("JAVA_DECOMPILER_TEST_JARS")
	if rawJars == "" {
		t.Skip("JAVA_DECOMPILER_TEST_JARS is not set")
	}
	full := os.Getenv("JAVA_DECOMPILER_FULL") == "1"

	for _, jarPath := range strings.Split(rawJars, ",") {
		jarPath = strings.TrimSpace(jarPath)
		if jarPath == "" {
			continue
		}
		t.Run(filepath.Base(jarPath), func(t *testing.T) {
			action := NewJavaDecompilerAction()
			t.Cleanup(action.ClearCache)

			start := time.Now()
			classes := walkJavaDecClasses(t, action, jarPath, 0)
			require.NotEmpty(t, classes)
			randomSample := randomJavaDecSample(classes, 5)
			t.Logf("listed %d Java sources in %s in %s; random samples: %v",
				len(classes), jarPath, time.Since(start), randomSample)

			preferred := []string{
				"com/sun/tools/javac/resources/compiler_zh_CN.java",
				"org/jd/gui/view/AboutView.java",
			}
			listed := make(map[string]bool, len(classes))
			for _, classPath := range classes {
				listed[classPath] = true
			}

			verifyPaths := make([]string, 0, len(preferred)+len(randomSample))
			verifySet := make(map[string]bool, len(preferred)+len(randomSample))
			appendPath := func(p string) {
				if listed[p] && !verifySet[p] {
					verifyPaths = append(verifyPaths, p)
					verifySet[p] = true
				}
			}
			for _, wanted := range preferred {
				appendPath(wanted)
			}
			for _, sampled := range randomSample {
				appendPath(sampled)
			}
			if full {
				for _, classPath := range classes {
					appendPath(classPath)
				}
			}

			// Random samples must carry a Java declaration. javajive emits
			// comment-only placeholders for some synthetic inner classes, so
			// draw further candidates until five real sources are sampled.
			randomSet := make(map[string]bool, len(randomSample))
			randomData := make(map[string][]byte, 5)
			sampledOrder := make([]string, 0, 5)
			takeSample := func(p string) {
				if len(sampledOrder) >= 5 || !listed[p] || randomSet[p] {
					return
				}
				randomSet[p] = true
				_, data := readJavaDecClass(t, action, jarPath, p)
				text := assertJavaSource(t, p, data)
				if !javaSourceHasDeclaration(text) {
					t.Logf("sample %s is a decompiler placeholder; drawing another", p)
					return
				}
				assertJavaSourceKeywords(t, p, text)
				randomData[p] = data
				sampledOrder = append(sampledOrder, p)
			}
			for _, sampled := range randomSample {
				takeSample(sampled)
			}
			if len(sampledOrder) < 5 {
				shuffled := append([]string(nil), classes...)
				r := rand.New(rand.NewSource(20261119))
				r.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
				for _, candidate := range shuffled {
					takeSample(candidate)
					if len(sampledOrder) >= 5 {
						break
					}
				}
			}
			if len(classes) >= 5 {
				require.GreaterOrEqual(t, len(sampledOrder), 5, "must sample five Java sources with declarations")
			}
			t.Logf("verified samples: %v", sampledOrder)

			verified := 0
			for _, classPath := range verifyPaths {
				resource, data := readJavaDecClass(t, action, jarPath, classPath)
				require.Equal(t, "class", resource.ResourceType)
				text := string(data)
				if cached, ok := randomData[classPath]; ok {
					data = cached
					text = string(cached)
				}
				assertJavaSource(t, classPath, data)
				switch {
				case strings.HasSuffix(classPath, "compiler_zh_CN.java"):
					require.Contains(t, text, "无法直接访问")
				case strings.HasSuffix(classPath, "AboutView.java"):
					require.Contains(t, text, "Copyright ©")
				}
				if verified == 0 {
					lines := strings.SplitN(text, "\n", 4)
					t.Logf("first source head: %q", lines)
				}
				verified++
			}
			t.Logf("verified %d Java sources from %s in %s", verified, jarPath, time.Since(start))

			if !full {
				return
			}
			contents, names := exportJavaDecZip(t, action, jarPath)
			for _, name := range names {
				require.False(t, strings.HasSuffix(strings.TrimSuffix(name, "/"), ".class"),
					"export must not contain raw class entries: %s", name)
			}
			require.NotEmpty(t, contents, "export must contain Java sources")
			exportedSample := sampledOrder
			for _, classPath := range exportedSample {
				exportedPath := strings.TrimSuffix(classPath, ".java") + ".java"
				data, ok := contents[exportedPath]
				if !ok {
					continue
				}
				assertJavaSource(t, exportedPath, data)
			}
			t.Logf("exported %d files from %s in %s", len(contents), jarPath, time.Since(start))
		})
	}
}

// TestJavaDecompilerSourceViewExportAndDedup covers the two cases the plain
// listing used to hide: a jar that ships both X.class and X.java, and a nested
// jar whose bytecode must be exported as Java source.
func TestJavaDecompilerSourceViewExportAndDedup(t *testing.T) {
	probe := readEncodingProbeClass(t)
	nestedJar := makeTestJarBytes(t, map[string][]byte{
		"EncodingProbe.class": probe,
	})
	jarPath := writeTestJar(t, "source-view.jar", map[string][]byte{
		"lib/dep.jar":         nestedJar,
		"EncodingProbe.class": probe,
		"EncodingProbe.java":  []byte("public class EncodingProbe { /* real source */ }\n"),
	})

	action := NewJavaDecompilerAction()
	t.Cleanup(action.ClearCache)

	rootEntries := listJavaDecDir(t, action, jarPath, ".")
	var rootSource *ypb.YakURLResource
	for _, entry := range rootEntries {
		if entry.ResourceName == "EncodingProbe.class" {
			t.Fatalf("root listing must not expose raw class entries: %s", entry.ResourceName)
		}
		if entry.ResourceName == "EncodingProbe.java" {
			rootSource = entry
		}
	}
	require.NotNil(t, rootSource, "root listing must contain EncodingProbe.java")
	require.Equal(t, "java-source", rootSource.VerboseType)
	require.Equal(t, "EncodingProbe.class", extraValue(rootSource, "originalPath"))

	// The bytecode entry wins over the same-named real source.
	_, rootData := readJavaDecClass(t, action, jarPath, rootSource.Path)
	rootText := assertJavaSource(t, rootSource.Path, rootData)
	require.Contains(t, rootText, "中文字符串常量")

	nestedEntries := listJavaDecDir(t, action, jarPath, "lib/dep.jar")
	require.Len(t, nestedEntries, 1)
	require.Equal(t, "EncodingProbe.java", nestedEntries[0].ResourceName)
	require.Equal(t, "lib/dep.jar/EncodingProbe.class", extraValue(nestedEntries[0], "originalPath"))
	_, nestedData := readJavaDecClass(t, action, jarPath, nestedEntries[0].Path)
	assertJavaSource(t, nestedEntries[0].Path, nestedData)
	require.Equal(t, rootData, nestedData)

	contents, names := exportJavaDecZip(t, action, jarPath)
	t.Logf("nested export entries: %v", names)
	for _, name := range names {
		require.False(t, strings.HasSuffix(strings.TrimSuffix(name, "/"), ".class"),
			"export must not contain raw class entries: %v", names)
	}
	require.Contains(t, contents, "EncodingProbe.java")
	require.Contains(t, contents, "lib/dep.jar/EncodingProbe.java")
	assertJavaSource(t, "EncodingProbe.java", contents["EncodingProbe.java"])
	assertJavaSource(t, "lib/dep.jar/EncodingProbe.java", contents["lib/dep.jar/EncodingProbe.java"])
	require.Equal(t, rootData, contents["EncodingProbe.java"])
}

// TestJavaDecompilerDirectorySourceView covers a directory root that holds
// loose class files and nested archives; both must read as Java source.
func TestJavaDecompilerDirectorySourceView(t *testing.T) {
	probe := readEncodingProbeClass(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "EncodingProbe.class"), probe, 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "lib"), 0o755))
	nested := makeTestJarBytes(t, map[string][]byte{"EncodingProbe.class": probe})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lib", "dep.jar"), nested, 0o644))

	action := NewJavaDecompilerAction()
	t.Cleanup(action.ClearCache)

	rootEntries := listJavaDecDir(t, action, dir, ".")
	var looseSource *ypb.YakURLResource
	for _, entry := range rootEntries {
		require.False(t, strings.HasSuffix(entry.ResourceName, ".class"),
			"directory listing must not expose raw class entries: %s", entry.ResourceName)
		if entry.ResourceName == "EncodingProbe.java" {
			looseSource = entry
		}
	}
	require.NotNil(t, looseSource, "directory listing must contain EncodingProbe.java")

	_, looseData := readJavaDecClass(t, action, dir, looseSource.Path)
	looseText := assertJavaSource(t, looseSource.Path, looseData)
	require.Contains(t, looseText, "中文字符串常量")

	nestedEntries := listJavaDecDir(t, action, dir, "lib/dep.jar")
	require.Len(t, nestedEntries, 1)
	require.Equal(t, "EncodingProbe.java", nestedEntries[0].ResourceName)

	_, nestedData := readJavaDecClass(t, action, dir, nestedEntries[0].Path)
	nestedText := assertJavaSource(t, nestedEntries[0].Path, nestedData)
	require.Contains(t, nestedText, "中文字符串常量")
	require.Equal(t, looseData, nestedData)

	contents, names := exportJavaDecZip(t, action, dir)
	t.Logf("directory export entries: %v", names)
	for _, name := range names {
		require.False(t, strings.HasSuffix(strings.TrimSuffix(name, "/"), ".class"),
			"directory export must not contain raw class entries: %v", names)
	}
	require.Contains(t, contents, "EncodingProbe.java")
	require.Contains(t, contents, "lib/dep.jar/EncodingProbe.java")
	require.Equal(t, looseData, contents["EncodingProbe.java"])
	require.Equal(t, nestedData, contents["lib/dep.jar/EncodingProbe.java"])
}
