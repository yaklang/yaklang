package java_decompiler

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"io"
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
	fixture := filepath.Join("..", "..", "cmd", "yakcmds", "testdata", "java_encoding", "EncodingProbe.class")
	raw, err := os.ReadFile(fixture)
	require.NoError(t, err)

	jarPath := filepath.Join(t.TempDir(), "encoding-probe.jar")
	f, err := os.Create(jarPath)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	w, err := zw.Create("EncodingProbe.class")
	require.NoError(t, err)
	_, err = w.Write(raw)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
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
			if res.ResourceType == "dir" {
				if !seenDir[res.Path] {
					seenDir[res.Path] = true
					queue = append(queue, res.Path)
				}
				continue
			}
			if strings.HasSuffix(res.ResourceName, ".class") {
				classes = append(classes, res.Path)
				if limit > 0 && len(classes) >= limit {
					break
				}
			}
		}
	}
	return classes
}

// TestJavaDecompilerClassRouteReturnsSourceFromSmallJar is the fast regression
// test: a one-class jar must serve decompiled Java source from /class.
func TestJavaDecompilerClassRouteReturnsSourceFromSmallJar(t *testing.T) {
	jarPath := createEncodingProbeJar(t)
	action := NewJavaDecompilerAction()
	t.Cleanup(action.ClearCache)

	entries := listJavaDecDir(t, action, jarPath, ".")
	require.NotEmpty(t, entries)
	var listed []string
	for _, entry := range entries {
		listed = append(listed, entry.ResourceName)
	}
	t.Logf("small jar root listing: %v", listed)
	var classEntry *ypb.YakURLResource
	for _, entry := range entries {
		require.NotEqual(t, ".java", filepath.Ext(entry.ResourceName), "listing must keep the .class navigation name")
		if entry.ResourceName == "EncodingProbe.class" {
			classEntry = entry
		}
	}
	require.NotNil(t, classEntry, "root listing must contain EncodingProbe.class")
	require.Equal(t, "file", classEntry.ResourceType)
	require.Equal(t, "/class", classEntry.Url.Path)

	resource, data := readJavaDecClass(t, action, jarPath, classEntry.Path)
	require.Equal(t, "class", resource.ResourceType)
	require.Equal(t, "decompiled-java-class", resource.VerboseType)

	text := assertJavaSource(t, classEntry.Path, data)
	require.Contains(t, text, "中文字符串常量")
	require.Contains(t, text, "日本語の文字列")
	require.Contains(t, text, "한국어 문자열")
	require.Contains(t, text, "café naïve © ®")
	require.NotContains(t, text, "\uFFFD")

	exportResp, err := action.Get(newJavaDecTestURL(t, "/export",
		&ypb.KVPair{Key: "jar", Value: jarPath},
	))
	require.NoError(t, err)
	require.Len(t, exportResp.Resources, 1)
	var zipPath string
	for _, kv := range exportResp.Resources[0].Extra {
		if kv.Key == "path" {
			zipPath = kv.Value
		}
	}
	require.NotEmpty(t, zipPath)
	zr, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer zr.Close()

	var exported []byte
	for _, f := range zr.File {
		if f.Name != "EncodingProbe.java" {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		exported, err = io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
	}
	require.NotEmpty(t, exported, "export must contain decompiled EncodingProbe.java")
	assertJavaSource(t, "EncodingProbe.java", exported)
}

// TestJavaDecompilerClassRouteRealJars samples the two reported jars. Set
// JAVA_DECOMPILER_TEST_JARS to a comma-separated list to enable it.
// Set JAVA_DECOMPILER_FULL=1 to verify every class in every jar.
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
			sample := classes
			if len(sample) > 5 {
				sample = sample[:5]
			}
			t.Logf("listed %d classes in %s in %s; samples: %v", len(classes), jarPath, time.Since(start), sample)

			verifyLimit := 5
			if full {
				verifyLimit = 0
			}
			preferred := []string{
				"com/sun/tools/javac/resources/compiler_zh_CN.class",
				"org/jd/gui/view/AboutView.class",
			}
			listed := make(map[string]bool, len(classes))
			for _, classPath := range classes {
				listed[classPath] = true
			}
			ordered := make([]string, 0, len(classes))
			orderedSet := make(map[string]bool, len(classes))
			for _, wanted := range preferred {
				if listed[wanted] && !orderedSet[wanted] {
					ordered = append(ordered, wanted)
					orderedSet[wanted] = true
				}
			}
			for _, classPath := range classes {
				if !orderedSet[classPath] {
					ordered = append(ordered, classPath)
					orderedSet[classPath] = true
				}
			}
			verified := 0
			for _, classPath := range ordered {
				if verifyLimit > 0 && verified >= verifyLimit {
					break
				}
				resource, data := readJavaDecClass(t, action, jarPath, classPath)
				require.Equal(t, "class", resource.ResourceType)
				text := assertJavaSource(t, classPath, data)
				switch {
				case strings.HasSuffix(classPath, "compiler_zh_CN.class"):
					require.Contains(t, text, "无法直接访问")
				case strings.HasSuffix(classPath, "AboutView.class"):
					require.Contains(t, text, "Copyright ©")
				}
				if verified == 0 {
					lines := strings.SplitN(text, "\n", 4)
					t.Logf("first source head: %q", lines)
				}
				verified++
			}
			t.Logf("verified %d classes from %s in %s", verified, jarPath, time.Since(start))
		})
	}
}
