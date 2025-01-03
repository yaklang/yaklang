package yakgrpc

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServerExportsPlugins(t *testing.T) {
	client, _ := NewLocalClient()
	uid := uuid.New().String()

	name1, clearFunc, err := yakit.CreateTemporaryYakScriptEx("yak", "hello 1; "+uid, uid)
	require.NoError(t, err)
	defer clearFunc()
	name2, clearFunc2, err := yakit.CreateTemporaryYakScriptEx("yak", "hello 2; "+uid, uid)
	require.NoError(t, err)
	defer clearFunc2()
	stream, err := client.ExportYakScriptStream(
		context.Background(),
		&ypb.ExportYakScriptStreamRequest{
			Filter: &ypb.QueryYakScriptRequest{
				Keyword:  uid,
				IsIgnore: true,
			},
			OutputFilename: "",
			Password:       "",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	outputFile := ""
	for {
		client, err := stream.Recv()
		if err != nil {
			break
		}
		if client.IsMessage {
			data := gjson.ParseBytes(client.Message).Get("content").Get("data")
			pathName := gjson.Parse(data.Str).Get("path").Str
			if pathName != "" {
				outputFile = pathName
			}
		}
	}
	if outputFile == "" {
		t.Fatal("output file is empty")
	}
	if utils.GetFirstExistedFile(outputFile) == "" {
		t.Fatal("output file not found")
	}

	yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), name1)
	yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), name2)

	stream2, err := client.ImportYakScriptStream(context.Background(), &ypb.ImportYakScriptStreamRequest{
		Filename: outputFile,
	})
	for {
		client, err := stream2.Recv()
		if err != nil {
			break
		}
		if client.IsMessage {
			log.Infof("message: %s", client.Message)
		}
	}
	t1, _ := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), name1)
	t2, _ := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), name2)
	assert.NotNil(t, t1)
	assert.NotNil(t, t2)

	yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), name1)
	yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), name2)
}

func TestServerExportsPlugins_Enc(t *testing.T) {
	client, _ := NewLocalClient()
	uid := uuid.New().String()

	name1, clearFunc, err := yakit.CreateTemporaryYakScriptEx("yak", "hello 1; "+uid, uid)
	require.NoError(t, err)
	defer clearFunc()
	name2, clearFunc2, err := yakit.CreateTemporaryYakScriptEx("yak", "hello 2; "+uid, uid)
	require.NoError(t, err)
	defer clearFunc2()
	assert.NotEmpty(t, name1)
	assert.NotEmpty(t, name2)

	password := utils.RandSecret(6)

	stream, err := client.ExportYakScriptStream(
		context.Background(),
		&ypb.ExportYakScriptStreamRequest{
			Filter: &ypb.QueryYakScriptRequest{
				Keyword:  uid,
				IsIgnore: true,
			},
			OutputFilename: "",
			Password:       password,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	outputFile := ""
	for {
		client, err := stream.Recv()
		if err != nil {
			break
		}
		if client.IsMessage {
			fmt.Println(string(client.Message))
			data := gjson.ParseBytes(client.Message).Get("content").Get("data")
			pathName := gjson.Parse(data.Str).Get("path").Str
			if pathName != "" {
				outputFile = pathName
			}
		}
	}
	if outputFile == "" {
		t.Fatal("output file is empty")
	}
	if utils.GetFirstExistedFile(outputFile) == "" {
		t.Fatal("output file not found")
	}

	yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), name1)
	yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), name2)

	stream2, _ := client.ImportYakScriptStream(context.Background(), &ypb.ImportYakScriptStreamRequest{
		Filename: outputFile,
		Password: password,
	})
	for {
		client, err := stream2.Recv()
		if err != nil {
			break
		}
		if client.IsMessage {
			log.Infof("message: %s", client.Message)
		}
	}
	t1, _ := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), name1)
	t2, _ := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), name2)
	assert.NotNil(t, t1)
	assert.NotNil(t, t2)

	yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), name1)
	yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), name2)
}

func TestServerImportsPlugins(t *testing.T) {
	client, _ := NewLocalClient()

	content := "hello 1; " + uuid.NewString()
	name, clearFunc, err := yakit.CreateTemporaryYakScriptEx("yak", content)
	t.Cleanup(clearFunc)

	createYakOutputZip := func() (string, string) {
		newContent := uuid.NewString()
		script, err := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), name)
		require.NoError(t, err)
		script.ID = 0
		require.Contains(t, script.Content, content)
		script.Content = newContent

		scriptRaw, err := json.Marshal(script)
		require.NoError(t, err)
		// create output plugin file
		path := t.TempDir() + "/final.zip"
		fp, err := os.Create(path)
		defer fp.Close()

		zipWriter := zip.NewWriter(fp)
		fileName := uuid.NewString() + ".json"
		fileSaver, err := zipWriter.Create(fileName)
		require.NoError(t, err)
		_, err = fileSaver.Write(scriptRaw)
		require.NoError(t, err)
		var output = make([]map[string]interface{}, 0, 64)
		output = append(output, map[string]any{
			"filename":    fileName,
			"script_name": script.ScriptName,
		})
		err = zipWriter.Flush()
		require.NoError(t, err)
		writer, err := zipWriter.Create("meta.json")
		require.NoError(t, err)
		raw, err := json.Marshal(output)
		require.NoError(t, err)
		_, err = writer.Write(raw)
		require.NoError(t, err)
		err = zipWriter.Close()
		require.NoError(t, err)
		return path, newContent
	}
	outputFile, newContent := createYakOutputZip()
	importStream, err := client.ImportYakScriptStream(context.Background(), &ypb.ImportYakScriptStreamRequest{
		Filename: outputFile,
	})
	require.NoError(t, err)
	for {
		client, err := importStream.Recv()
		if err != nil {
			break
		}
		if client.IsMessage {
			log.Infof("message: %s", client.Message)
		}
	}

	newScript, _ := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), name)
	require.NotNil(t, newScript)
	require.Contains(t, newScript.Content, newContent)
	require.NotContains(t, newScript.Content, content)
}
