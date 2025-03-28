package java_decompiler

import (
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReadNestedJarDirectory(t *testing.T) {
	jarPath := "/Users/z3/Downloads/h5-graph-lite.jar"
	dirPath := "BOOT-INF/lib/antlr-2.7.7.jar/antlr/actions/cpp"
	rootURL, _ := CreateUrlFromString("javadec:///jar-aifix?jar=" + jarPath + "&dir=" + dirPath)
	rootParams := &ypb.RequestYakURLParams{
		Url:    rootURL,
		Method: "GET",
	}
	rootResp, err := NewJavaDecompilerAction().Get(rootParams)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range rootResp.Resources {
		println(res.Path)
	}
}
func TestReadNestedJarClass(t *testing.T) {
	jarPath := "/Users/z3/Downloads/h5-graph-lite.jar"
	dirPath := "BOOT-INF/lib/antlr-2.7.7.jar/antlr/actions/cpp/ActionLexer.class"
	rootURL, _ := CreateUrlFromString("javadec:///class-aifix?jar=" + jarPath + "&class=" + dirPath)
	rootParams := &ypb.RequestYakURLParams{
		Url:    rootURL,
		Method: "GET",
	}
	rootResp, err := NewJavaDecompilerAction().Get(rootParams)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range rootResp.Resources {
		if res.ResourceType == "file" {
			t.Log(res.Path)
		}
	}
}
