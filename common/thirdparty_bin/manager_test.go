package thirdparty_bin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManager_Start 验证 Manager 端到端 Register + Install 链路
// 完全离线: 用 httptest.Server 模拟下载源, NewManager 用临时目录
// 关键词: Manager Register Install, 离线集成测试, httptest, 不依赖 DefaultManager
func TestManager_Start(t *testing.T) {
	binaryName := "test-bin-mgr"
	binaryContent := []byte("#!/bin/sh\necho mgr-test\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(binaryContent)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(binaryContent)
	}))
	defer srv.Close()

	downloadDir := t.TempDir()
	installDir := t.TempDir()

	manager, err := NewManager(downloadDir, installDir)
	require.NoError(t, err, "NewManager must succeed")
	require.NotNil(t, manager, "manager must not be nil")

	desc := &BinaryDescriptor{
		Name:        binaryName,
		Description: "offline test binary",
		Version:     "latest",
		InstallType: "bin",
		DownloadInfoMap: map[string]*DownloadInfo{
			"*": {
				URL:     srv.URL + "/" + binaryName,
				BinPath: binaryName,
			},
		},
	}

	require.NoError(t, manager.Register(desc), "register must succeed")

	require.NoError(t, manager.Install(binaryName, &InstallOptions{
		Force:   true,
		Context: context.Background(),
	}), "install must succeed via httptest server")

	installedPath := filepath.Join(installDir, binaryName)
	got, err := os.ReadFile(installedPath)
	require.NoError(t, err, "installed binary must be readable at %s", installedPath)
	assert.Equal(t, binaryContent, got, "installed bytes must match server payload")
}
