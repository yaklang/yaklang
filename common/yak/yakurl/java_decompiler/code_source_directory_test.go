package java_decompiler

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestJavaDecompilerAction_DirectoryCodeSource(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "javadec-dir-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	jarPath := filepath.Join(tempDir, "sample.jar")
	jarFile, err := os.Create(jarPath)
	require.NoError(t, err)
	zipWriter := zip.NewWriter(jarFile)
	classWriter, err := zipWriter.Create("com/example/Hello.class")
	require.NoError(t, err)
	_, err = classWriter.Write([]byte{0xca, 0xfe, 0xba, 0xbe})
	require.NoError(t, err)
	require.NoError(t, zipWriter.Close())
	require.NoError(t, jarFile.Close())

	action := NewJavaDecompilerAction()
	defer action.ClearCache()

	listURL, err := CreateUrlFromString("javadec:///jar-aifix")
	require.NoError(t, err)
	listURL.Query = []*ypb.KVPair{
		{Key: "jar", Value: tempDir},
		{Key: "dir", Value: "."},
	}
	listResp, err := action.Get(&ypb.RequestYakURLParams{Url: listURL, Method: "GET"})
	require.NoError(t, err)
	require.NotEmpty(t, listResp.Resources)

	var foundJar bool
	for _, res := range listResp.Resources {
		if res.ResourceName == "sample.jar" {
			foundJar = true
			require.True(t, res.HaveChildrenNodes)
		}
	}
	require.True(t, foundJar, "directory listing should contain sample.jar")

	innerURL, err := CreateUrlFromString("javadec:///jar-aifix")
	require.NoError(t, err)
	innerURL.Query = []*ypb.KVPair{
		{Key: "jar", Value: tempDir},
		{Key: "dir", Value: "sample.jar"},
	}
	innerResp, err := action.Get(&ypb.RequestYakURLParams{Url: innerURL, Method: "GET"})
	require.NoError(t, err)
	require.NotEmpty(t, innerResp.Resources)
}
