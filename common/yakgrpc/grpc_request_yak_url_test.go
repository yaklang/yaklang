package yakgrpc

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func ExtraKVPairsToMap(pairs []*ypb.KVPair) map[string]string {
	m := make(map[string]string)
	for _, pair := range pairs {
		m[pair.Key] = pair.Value
	}
	return m
}

func TestGRPCMUSTPASS_RequestYakURL_Get(t *testing.T) {
	t.Run("fs-list", func(t *testing.T) {
		p := "/"
		if runtime.GOOS == "windows" {
			p = "C:\\"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)

		resources, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: http.MethodGet,
			Url: &ypb.YakURL{
				FromRaw: fmt.Sprintf("file://%s?op=list", p),
			},
		})
		require.NoError(t, err)
		t.Logf("resources len: %d", resources.Total)
		require.Greater(t, int(resources.Total), 0, "resources should not be empty")
	})

	t.Run("fs-get-detect-plain-text-text", func(t *testing.T) {
		fh, err := os.CreateTemp("", "yak-test-fs")
		require.NoError(t, err)
		defer os.Remove(fh.Name())
		_, err = fh.WriteString("hello")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)
		resources, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: http.MethodGet,
			Url: &ypb.YakURL{
				FromRaw: fmt.Sprintf("file://%s?detectPlainText=true", fh.Name()),
			},
		})
		require.NoError(t, err)
		require.Len(t, resources.Resources, 1)
		resource := resources.Resources[0]
		extra := ExtraKVPairsToMap(resource.GetExtra())
		require.Equal(t, "true", extra["IsPlainText"])
	})

	t.Run("fs-get-detect-plain-text-image", func(t *testing.T) {
		fh, err := os.CreateTemp("", "yak-test-fs")
		require.NoError(t, err)
		defer os.Remove(fh.Name())
		_, err = fh.WriteString("GIF89a") // GIF magic number
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)
		resources, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: http.MethodGet,
			Url: &ypb.YakURL{
				FromRaw: fmt.Sprintf("file://%s?detectPlainText=true", fh.Name()),
			},
		})
		require.NoError(t, err)
		require.Len(t, resources.Resources, 1)
		resource := resources.Resources[0]
		extra := ExtraKVPairsToMap(resource.GetExtra())
		require.Equal(t, "false", extra["IsPlainText"])
	})
}

func TestGRPCMUSTPASS_RequestYakURL_Put(t *testing.T) {
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
				FromRaw: fmt.Sprintf("file://%s?type=file", fileName),
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
				FromRaw: fmt.Sprintf("file://%s?type=dir", dirName),
			},
		})
		require.NoError(t, err)
		require.Equal(t, res.GetResources()[0].Path, dirName)
		_, err = os.Stat(dirName)
		require.NoError(t, err)
		os.Remove(dirName)
	})

	t.Run("fs-put-dir-duplicate-name-should-error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := NewLocalClient()
		require.NoError(t, err)

		dirName := filepath.Join(os.TempDir(), utils.RandStringBytes(5))
		err = os.Mkdir(dirName, 0o755)
		require.NoError(t, err)
		defer os.RemoveAll(dirName)

		_, err = client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "PUT",
			Url: &ypb.YakURL{
				FromRaw: fmt.Sprintf("file://%s?type=dir", dirName),
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "path exists")
	})
}

func TestGRPCMUSTPASS_RequestYakURL_Post(t *testing.T) {
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
				FromRaw: fmt.Sprintf("file://%s?op=content", fileName),
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
		defer func() {
			os.RemoveAll(fileName)
			os.RemoveAll(newName)
		}()
		res, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "POST",
			Url: &ypb.YakURL{
				FromRaw: fmt.Sprintf("file://%s?op=rename&newname=%s", fileName, newName),
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
		err = os.Mkdir(dirName, 0o755)
		require.NoError(t, err)
		newName := filepath.Join(os.TempDir(), utils.RandStringBytes(5))
		defer func() {
			os.RemoveAll(dirName)
			os.RemoveAll(newName)
		}()
		res, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "POST",
			Url: &ypb.YakURL{
				FromRaw: fmt.Sprintf("file://%s?op=rename&newname=%s", dirName, newName),
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

func TestGRPCMUSTPASS_RequestYakURL_Delete(t *testing.T) {
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
				FromRaw: fmt.Sprintf("file://%s", fileName),
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
		err = os.Mkdir(fileName, 0o755)
		require.NoError(t, err)
		_, err = client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: "DELETE",
			Url: &ypb.YakURL{
				FromRaw: fmt.Sprintf("file://%s", fileName),
			},
		})
		require.NoError(t, err)
		exists, err := utils.PathExists(fileName)
		require.False(t, exists)
		require.NoError(t, err)
		os.Remove(fileName)
	})
}

// TestGRPCMUSTPASS_RequestYakURL_SSARiskRuleTreeSlashInRuleName inserts a synthetic SSA risk whose
// FromRule contains '/' and checks RequestYakURL (same entry as Yakit ipcRenderer.invoke('RequestYakURL')).
// Backend maps ASCII '/' to fullwidth '／' in rule ResourceName and Path segments so clients can split on '/'.
func TestGRPCMUSTPASS_RequestYakURL_SSARiskRuleTreeSlashInRuleName(t *testing.T) {
	slashRule := "【tmp】检测math/rand与jwt-go/路径截断回归/" + uuid.NewString()
	wantDisplay := strings.ReplaceAll(slashRule, "/", "\uFF0F")
	programName := "grpc_ssarisk_slash_" + uuid.NewString()
	db := ssadb.GetDB()
	err := yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:     programName,
		CodeSourceUrl:   fmt.Sprintf("/%s/sub/pkg/x.go", programName),
		FunctionName:    "F",
		Title:           "slash-rule-title",
		TitleVerbose:    "slash-rule-title-verbose",
		FromRule:        slashRule,
		ResultID:        999001,
		Variable:        "v",
		Index:           1,
		RiskFeatureHash: programName + "-slash-rule-feature",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, yakit.DeleteSSARisks(db, &ypb.SSARisksFilter{ProgramName: []string{programName}}))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := NewLocalClient()
	require.NoError(t, err)

	resp, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
		Method: http.MethodGet,
		Url: &ypb.YakURL{
			Schema: "ssarisk",
			Path:   "/",
			Query: []*ypb.KVPair{
				{Key: "type", Value: "rule"},
				{Key: "program", Value: programName},
			},
		},
	})
	require.NoError(t, err)
	require.Greater(t, len(resp.GetResources()), 0)

	var ruleRes *ypb.YakURLResource
	for _, r := range resp.GetResources() {
		if r.GetResourceType() == "rule" && r.GetResourceName() == wantDisplay {
			ruleRes = r
			break
		}
	}
	require.NotNil(t, ruleRes, "no rule row with slash-mapped FromRule; Resources=%+v", resp.GetResources())
	require.NotContains(t, ruleRes.GetResourceName(), "math/rand")
	require.Contains(t, ruleRes.GetResourceName(), "math\uFF0Frand")
	require.Contains(t, ruleRes.GetPath(), wantDisplay)
}
