package coreplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"strconv"
	"strings"
	"testing"
)

func TestSsaSearch(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("/var/www/html/1.php", `<?php
$a = "a1a";
$b = "funcA(";
function funcA(){}
funcA();
`)
	fs.AddFile("/var/www/html/2.php", `<?php
$b = "b2b";
function funcB(){}
`)
	progName := uuid.NewString()
	prog, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(fs),
		ssaapi.WithLanguage(ssaapi.PHP),
		ssaapi.WithProgramName(progName))
	require.NoError(t, err)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}()
	prog.Show()
	client, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)

	pluginName := "SyntaxFlow Searcher"
	initDB.Do(func() {
		yakit.InitialDatabase()
	})
	codeBytes := GetCorePluginData(pluginName)
	require.NotNilf(t, codeBytes, "无法从bindata获取: %v", pluginName)
	check := func(t *testing.T, kind, rule string, check func(*ssadb.AuditNode)) {
		stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
			Code:       string(codeBytes),
			PluginType: "yak",
			ExecParams: []*ypb.KVPair{
				{
					Key:   "kind",
					Value: kind,
				},
				{
					Key:   "rule",
					Value: rule,
				},
				{
					Key:   "progName",
					Value: progName,
				},
				{
					Key:   "fuzz",
					Value: "true",
				},
			},
		})
		require.NoError(t, err)
		runtimeId := ""
		resultId := -1
		result := new(msg)
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
				fmt.Println("raw msg: ", string(rawMsg))
				json.Unmarshal(rawMsg, &result)
				if result.Content.Level == "json" && result.Content.Data != "" {
					id, err := strconv.Atoi(result.Content.Data)
					if err != nil {
						log.Errorf("invalid result id: %v", string(rawMsg))
						continue
					}
					resultId = id
					for node := range ssadb.YieldAuditNodeByResultId(ssadb.GetDB(), uint(resultId)) {
						check(node)
					}
					break
				}
			}
		}
	}
	t.Run("check kind", func(t *testing.T) {
		check(t, "function", "funcA", func(node *ssadb.AuditNode) {
			ircode := ssadb.GetIrCodeById(ssadb.GetDB(), node.IRCodeID)
			require.True(t, ircode.Opcode == 26)
			require.Contains(t, ircode.Name, "funcA")
		})
		check(t, "const", "1", func(node *ssadb.AuditNode) {
			ircode := ssadb.GetIrCodeById(ssadb.GetDB(), node.IRCodeID)
			require.True(t, ircode.Opcode == 5)
			require.Contains(t, ircode.String, "a1a")

		})
		check(t, "symbol", "funcA", func(node *ssadb.AuditNode) {
			ircode := ssadb.GetIrCodeById(ssadb.GetDB(), node.IRCodeID)
			require.True(t, ircode.Opcode == 26 || ircode.Opcode == 4)
		})
		check(t, "symbol", "a1a", func(node *ssadb.AuditNode) {
			ircode := ssadb.GetIrCodeById(ssadb.GetDB(), node.IRCodeID)
			require.True(t, ircode.Opcode == 5)
			require.Contains(t, ircode.String, "a1a")
		})
		check(t, "file", "2", func(node *ssadb.AuditNode) {
			fmt.Println(node.TmpValue)
			require.True(t, node.TmpValue == "var/www/html/2.php")
		})
		check(t, "all", "html", func(node *ssadb.AuditNode) {
			fmt.Println(node.TmpValue)
			require.True(t, node.TmpValue == "var/www/html/1.php" || node.TmpValue == "var/www/html/2.php")
		})
		funcA := false
		funcB := false
		check(t, "all", "func", func(node *ssadb.AuditNode) {
			ircode := ssadb.GetIrCodeById(ssadb.GetDB(), node.IRCodeID)
			if strings.Contains(ircode.Name, "funcA") {
				funcA = true
			}
			if strings.Contains(ircode.Name, "funcB") {
				funcB = true
			}
		})
		require.True(t, funcA && funcB)
		check(t, "all", "funcA(", func(node *ssadb.AuditNode) {
			ircode := ssadb.GetIrCodeById(ssadb.GetDB(), node.IRCodeID)
			require.True(t, ircode.Opcode == 4 || ircode.Opcode == 5)
		})
		check(t, "const", "funcA(", func(node *ssadb.AuditNode) {
			ircode := ssadb.GetIrCodeById(ssadb.GetDB(), node.IRCodeID)
			require.True(t, ircode.Opcode == 5)
		})
	})
}
