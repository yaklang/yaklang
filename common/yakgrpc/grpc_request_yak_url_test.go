package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRequestYakURLGet(t *testing.T) {
	t.Run("fs", func(t *testing.T) {
		p := "/"
		if runtime.GOOS == "windows" {
			p = "C:\\"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)

		resources, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "file",
				Path:   p,
				Query: []*ypb.KVPair{
					{
						Key:   "op",
						Value: "list",
					},
				},
			},
		})
		require.NoError(t, err)
		t.Logf("resources len: %d", resources.Total)
		require.Greater(t, int(resources.Total), 0, "resources should not be empty")
	})
}

func TestRequestYakURLPut(t *testing.T) {
	t.Run("fs-put-file", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)
		fileName := filepath.Join(os.TempDir(), utils.RandStringBytes(5))
		content := utils.RandStringBytes(20)
		res, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "PUT",
			Url: &ypb.YakURL{
				Schema: "file",
				Path:   fileName,
				Query: []*ypb.KVPair{
					{
						Key:   "type",
						Value: "file",
					},
				},
			},
			Body: []byte(content),
		})
		require.NoError(t, err)
		require.Equal(t, res.GetResources()[0].Path, fileName)
		readContent, err := os.ReadFile(fileName)
		require.NoError(t, err)
		require.Equal(t, content, string(readContent))
		os.Remove(fileName)
	})

	t.Run("fs-put-dir", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)
		dirName := filepath.Join(os.TempDir(), utils.RandStringBytes(5))
		res, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "PUT",
			Url: &ypb.YakURL{
				Schema: "file",
				Path:   dirName,
				Query: []*ypb.KVPair{
					{
						Key:   "type",
						Value: "dir",
					},
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, res.GetResources()[0].Path, dirName)
		_, err = os.Stat(dirName)
		require.NoError(t, err)
		os.Remove(dirName)
	})
}

func TestRequestYakURLPost(t *testing.T) {
	t.Run("fs-post-content", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)
		fileName := filepath.Join(os.TempDir(), utils.RandStringBytes(5))
		create, err := os.Create(fileName)
		require.NoError(t, err)
		create.Close()
		fileContent := utils.RandStringBytes(20)
		res, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "POST",
			Url: &ypb.YakURL{
				Schema: "file",
				Path:   fileName,
				Query: []*ypb.KVPair{
					{
						Key:   "op",
						Value: "content",
					},
				},
			},
			Body: []byte(fileContent),
		})
		require.NoError(t, err)
		require.Equal(t, res.GetResources()[0].Path, fileName)
		readContent, err := os.ReadFile(fileName)
		require.NoError(t, err)
		require.Equal(t, fileContent, string(readContent))
		os.Remove(fileName)
	})

	t.Run("fs-post-rename-file", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)
		fileName := filepath.Join(os.TempDir(), utils.RandStringBytes(5))
		create, err := os.Create(fileName)
		require.NoError(t, err)
		newName := filepath.Join(os.TempDir(), utils.RandStringBytes(5))
		create.Close()
		res, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "POST",
			Url: &ypb.YakURL{
				Schema: "file",
				Path:   fileName,
				Query: []*ypb.KVPair{
					{
						Key:   "op",
						Value: "rename",
					}, {
						Key:   "newname",
						Value: newName,
					},
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, res.GetResources()[0].Path, newName)
		exists, err := utils.PathExists(fileName)
		require.False(t, exists)
		require.NoError(t, err)
		exists, err = utils.PathExists(newName)
		require.True(t, exists)
		require.NoError(t, err)
		os.Remove(fileName)
		os.Remove(newName)
	})

	t.Run("fs-post-rename-dir", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)
		dirName := filepath.Join(os.TempDir(), utils.RandStringBytes(5))
		err = os.Mkdir(dirName, 0755)
		require.NoError(t, err)
		newName := filepath.Join(os.TempDir(), utils.RandStringBytes(5))
		res, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "POST",
			Url: &ypb.YakURL{
				Schema: "file",
				Path:   dirName,
				Query: []*ypb.KVPair{
					{
						Key:   "op",
						Value: "rename",
					}, {
						Key:   "newname",
						Value: newName,
					},
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, res.GetResources()[0].Path, newName)
		exists, err := utils.PathExists(dirName)
		require.False(t, exists)
		require.NoError(t, err)
		exists, err = utils.PathExists(newName)
		require.True(t, exists)
		require.NoError(t, err)
		os.Remove(dirName)
		os.Remove(newName)
	})
}

func TestRequestYakURLDelete(t *testing.T) {
	t.Run("fs-Delete-file", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)
		fileName := filepath.Join(os.TempDir(), utils.RandStringBytes(5))
		create, err := os.Create(fileName)
		require.NoError(t, err)
		create.Close()
		_, err = client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "DELETE",
			Url: &ypb.YakURL{
				Schema: "file",
				Path:   fileName,
				Query:  []*ypb.KVPair{},
			},
		})
		require.NoError(t, err)
		exists, err := utils.PathExists(fileName)
		require.False(t, exists)
		require.NoError(t, err)
		os.Remove(fileName)
	})

	t.Run("fs-Delete-dir", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)
		fileName := filepath.Join(os.TempDir(), utils.RandStringBytes(5))
		err = os.Mkdir(fileName, 0755)
		require.NoError(t, err)
		_, err = client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "DELETE",
			Url: &ypb.YakURL{
				Schema: "file",
				Path:   fileName,
				Query:  []*ypb.KVPair{},
			},
		})
		require.NoError(t, err)
		exists, err := utils.PathExists(fileName)
		require.False(t, exists)
		require.NoError(t, err)
		os.Remove(fileName)
	})
}
