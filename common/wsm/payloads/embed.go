package payloads

import (
	"embed"
	"encoding/hex"
	"fmt"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync"
)

//go:embed behinder/static/*
var behinderPayloads embed.FS

type Payload string

var (
	EchoGo          Payload = "EchoGo"
	BasicInfoGo     Payload = "BasicInfoGo"
	CmdGo           Payload = "CmdGo"
	RealCMDGo       Payload = "RealCMDGo"
	FileOperationGo Payload = "FileOperationGo"
)

var payloads sync.Once
var HexPayload = map[string]map[Payload]string{}

func init() {
	dirs, err := behinderPayloads.ReadDir("behinder/static")
	if err != nil {
		panic(err)
	}
	for _, i := range dirs {
		script := ""
		fileName := i.Name()
		if strings.HasSuffix(strings.ToLower(fileName), ".class") {
			script = ypb.ShellScript_JSP.String()
		} else if strings.HasSuffix(strings.ToLower(fileName), ".php") {
			script = ypb.ShellScript_PHP.String()
		} else if strings.HasSuffix(strings.ToLower(fileName), ".asp") {
			script = ypb.ShellScript_ASP.String()
		} else if strings.HasSuffix(strings.ToLower(fileName), ".dll") {
			script = ypb.ShellScript_ASPX.String()
		}
		payloadType := Payload(strings.Split(fileName, ".")[0])

		// https://github.com/golang/go/issues/45230
		raw, err := behinderPayloads.ReadFile(fmt.Sprintf("behinder/static/%s", i.Name()))
		if err != nil {
			panic(err)
		}
		if _, exists := HexPayload[script]; !exists {
			HexPayload[script] = make(map[Payload]string)
		}

		// 添加到 HexPayload
		HexPayload[script][payloadType] = hex.EncodeToString(raw)
	}
}
