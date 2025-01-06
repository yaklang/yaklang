package coreplugin

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSSAAutoDetective(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)

	pluginName := "SSA 项目探测"
	initDB.Do(func() {
		yakit.InitialDatabase()
	})

	codeBytes := GetCorePluginData(pluginName)
	require.NotNilf(t, codeBytes, "无法从bindata获取: %v", pluginName)

	check := func(t *testing.T, input string) programInfo {
		ctx, cancel := context.WithCancel(context.Background())
		stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
			Code:       string(codeBytes),
			PluginType: "yak",
			ExecParams: []*ypb.KVPair{
				{
					Key:   "target",
					Value: input,
				},
			},
		})
		require.NoError(t, err)

		var info programInfo
		var runtimeId string
		for {
			exec, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Warn(err)
			}
			if runtimeId == "" {
				runtimeId = exec.RuntimeID
			}
			if exec.IsMessage {
				rawMsg := exec.GetMessage()
				var msg msg
				json.Unmarshal(rawMsg, &msg)
				log.Infof("msg: %v", msg)
				if msg.Type == "log" && msg.Content.Level == "code" {
					cancel()
					json.Unmarshal([]byte(msg.Content.Data), &info)
					break
				}
			}
		}

		require.NotEmpty(t, runtimeId)
		return info
	}

	t.Run("check jar", func(t *testing.T) {
		jarPath, err := ssatest.GetJarFile()
		require.NoError(t, err)
		info := check(t, jarPath)
		log.Infof("info: %v", info)
		require.Equal(t, info.Language, "java")
		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "")
		require.NotNil(t, info.Info)
		require.Equal(t, info.Info.Kind, "jar")
		require.Equal(t, info.Info.LocalFile, jarPath)
	})

	t.Run("check zip", func(t *testing.T) {
		zipPath, err := ssatest.GetZipFile()
		require.NoError(t, err)
		info := check(t, zipPath)
		log.Infof("info: %v", info)
		require.Equal(t, info.Language, "java")
		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "")
		require.NotNil(t, info.Info)
		require.Equal(t, info.Info.Kind, "compression")
		require.Equal(t, info.Info.LocalFile, zipPath)
	})

	t.Run("check error path", func(t *testing.T) {
		dir := os.TempDir()
		dir = path.Join(dir, "not-exist", uuid.NewString())
		info := check(t, dir)

		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "fileNotFoundException")
	})

	t.Run("check unsupported file ", func(t *testing.T) {
		dir := os.TempDir()
		file := path.Join(dir, "test.txt")
		err := os.WriteFile(file, []byte("test"), 0644)
		require.NoError(t, err)

		info := check(t, file)
		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "fileTypeException")
	})

	t.Run("check git", func(t *testing.T) {
		url, err := ssatest.GetLocalGit()
		require.NoError(t, err)
		info := check(t, url)
		log.Infof("info: %v", info)
		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "languageNeedSelectException")
	})

	t.Run("check un access url", func(t *testing.T) {
		info := check(t, "http://127.0.0.1:7777/1123/5"+uuid.NewString())
		require.NotNil(t, info.Error)
		require.Equal(t, info.Error.Kind, "connectFailException")
	})
}

type msg struct {
	Type    string `json:"type"`
	Content struct {
		Level   string  `json:"level"`
		Data    string  `json:"data"`
		ID      string  `json:"id"`
		Process float64 `json:"progress"`
	}
}

type programInfo struct {
	ProgramName string `json:"program_name"`
	Language    string `json:"language"`
	Info        struct {
		Kind      string `json:"kind"`
		LocalFile string `json:"local_file"`
		URL       string `json:"url"`
	}
	Description string `json:"description"`
	FileCount   int    `json:"file_count"`
	Error       struct {
		Kind string `json:"kind"`
		Msg  string `json:"msg"`
	}
}
