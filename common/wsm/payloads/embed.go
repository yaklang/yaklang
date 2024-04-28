package payloads

import (
	"embed"
	"encoding/hex"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync"
)

//go:embed behinder/static/*
var Payloads embed.FS

//go:embed yakshell/static/*
var YakPayloads embed.FS

//go:embed yakshell/encrypt/*
var YakEncrypt embed.FS

//go:embed godzilla/static/payload_test.dll
var CshrapPayload []byte

type Payload string

func (p Payload) String() string {
	return string(p)
}

// 目前将fileOperation payload 全部放在一起会造成数据包太大
var (
	AllPayload          Payload = "AllPayloadGo"
	EchoGo              Payload = "EchoGo"
	BasicInfoGo         Payload = "BasicInfoGo"
	CmdGo               Payload = "CmdGo"
	RealCMDGo           Payload = "RealCMDGo" //不太一样 后续实现
	FileOperationGo     Payload = "FileOperationGo"
	CreateFile          Payload = "CreateFile"
	UploadFile          Payload = "UploadFile"
	CopyFileOrDir       Payload = "CopyFileOrDir"
	DeleteFileOrDir     Payload = "DeleteFileOrDir"
	DirInfo             Payload = "DirInfo"
	DownloadFile        Payload = "DownloadFile"
	Mkdir               Payload = "Mk_dir"
	ReadFile            Payload = "Read_File"
	ReNameFile          Payload = "RenameFile"
	WgetFile            Payload = "WgetFile"
	ZipEncode           Payload = "ZipEncode"
	ChmodFilePremission Payload = "ChmodFilePremission"
	ChmodTime           Payload = "ChmodTime"
	DbOperation         Payload = "DbOperation"
	CheckHash           Payload = "CheckHash"
	EvilCode            Payload = "EvilCode"
)

var payloads sync.Once
var HexPayload = map[string]map[Payload]string{}
var YakShellPayload = map[string]map[Payload]string{}

// EncryptPayload 加密payload
var EncryptPayload = map[string]map[string]string{}

func init() {
	dirs, err := Payloads.ReadDir("behinder/static")
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
		raw, err := Payloads.ReadFile(fmt.Sprintf("behinder/static/%s", i.Name()))
		if err != nil {
			panic(err)
		}
		if _, exists := HexPayload[script]; !exists {
			HexPayload[script] = make(map[Payload]string)
		}

		// 添加到 HexPayload
		HexPayload[script][payloadType] = hex.EncodeToString(raw)
	}

	DecryptFunc := func(raw []byte) ([]byte, error) {
		compress, err := utils.GzipDeCompress(raw)
		if err != nil {
			return nil, err
		}
		for i, b := range compress {
			compress[i] = b ^ byte(i)
		}
		return compress, nil
	}
	_ = DecryptFunc
	//将Yakit_payload
	dirs, err = YakPayloads.ReadDir("yakshell/static")
	if err != nil {
		panic(err)
	}
	for _, i := range dirs {
		script := ""
		fileName := i.Name()
		if strings.Contains(strings.ToLower(fileName), ".class") {
			script = ypb.ShellScript_JSP.String()
		} else if strings.Contains(strings.ToLower(fileName), ".php") {
			script = ypb.ShellScript_PHP.String()
		} else if strings.Contains(strings.ToLower(fileName), ".asp") {
			script = ypb.ShellScript_ASP.String()
		} else if strings.Contains(strings.ToLower(fileName), ".dll") {
			script = ypb.ShellScript_ASPX.String()
		}
		payloadType := Payload(strings.Split(fileName, ".")[0])
		// https://github.com/golang/go/issues/45230
		raw, err := YakPayloads.ReadFile(fmt.Sprintf("yakshell/static/%s", i.Name()))
		if err != nil {
			panic(err)
		}
		raw, err = DecryptFunc(raw)
		if err != nil {
			panic(err)
		}
		if _, exists := YakShellPayload[script]; !exists {
			YakShellPayload[script] = make(map[Payload]string)
		}
		// 添加到 HexPayload
		YakShellPayload[script][payloadType] = hex.EncodeToString(raw)
	}

	//将加密方式加入
	dir, err := YakEncrypt.ReadDir("yakshell/encrypt")
	if err != nil {
		panic(err)
	}
	for _, entry := range dir {
		script := ""
		fileName := entry.Name()
		if strings.Contains(strings.ToLower(fileName), ".class") {
			script = ypb.ShellScript_JSP.String()
		} else if strings.Contains(strings.ToLower(fileName), ".php") {
			script = ypb.ShellScript_PHP.String()
		} else if strings.Contains(strings.ToLower(fileName), ".asp") {
			script = ypb.ShellScript_ASP.String()
		} else if strings.Contains(strings.ToLower(fileName), ".dll") {
			script = ypb.ShellScript_ASPX.String()
		}
		enryptType := strings.Split(fileName, ".")[0]
		file, err := YakEncrypt.ReadFile(fmt.Sprintf("yakshell/encrypt/%s", entry.Name()))
		if err != nil {
			panic(err)
		}
		file, err = DecryptFunc(file)
		if err != nil {
			panic(err)
		}
		if _, exists := EncryptPayload[script]; !exists {
			EncryptPayload[script] = make(map[string]string)
		}
		all := strings.ReplaceAll(string(file), "<?", "")
		//读取进去的时候，是完整的php文件
		EncryptPayload[script][enryptType] = all
	}
}
