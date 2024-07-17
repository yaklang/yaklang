package yakgrpc

import (
	"context"
	"fmt"
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
