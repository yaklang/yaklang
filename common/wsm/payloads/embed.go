package payloads

import (
	"embed"
	"encoding/hex"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync"
)

// payloads.tar.gz 由构建期的 gzip-embed 生成（见 init.go 的 go:generate），
// 内容是 behinder/static、yakshell/static、yakshell/encrypt、godzilla/static
// 四个明文目录，条目路径保持与源码目录一致，因此二进制内不再出现明文 payload，
// 开发者仍然可以直接编辑原始脚本文件。
//
//go:embed payloads.tar.gz
var payloadsFS embed.FS

const payloadXorKey = "yaklang-payload-v1"

// payload 在压缩包内的目录布局，与源码目录保持一致。
const (
	behinderStaticDir  = "behinder/static"
	yakshellStaticDir  = "yakshell/static"
	yakshellEncryptDir = "yakshell/encrypt"
	godzillaStaticDir  = "godzilla/static"
)

// FS 是所有 wsm payload 的统一文件系统，路径相对 common/wsm/payloads 目录。
var FS = mustNewPayloadFS(&payloadsFS, "payloads.tar.gz")

func mustNewPayloadFS(fs *embed.FS, fileName string) *gzip_embed.PreprocessingEmbed {
	ins, err := gzip_embed.NewPreprocessingEmbedWithXORKey(fs, fileName, true, []byte(payloadXorKey))
	if err != nil {
		panic(fmt.Sprintf("init payload fs %s failed: %v", fileName, err))
	}
	return ins
}

// ReadGodzillaPayload 读取打包在 payloads.tar.gz 中的 Godzilla 静态 payload。
func ReadGodzillaPayload(name string) ([]byte, error) {
	return FS.ReadFile(godzillaStaticDir + "/" + name)
}

// CshrapPayload 是 Godzilla ASPX payload 在内存中恢复后的原始字节。
var CshrapPayload = func() []byte {
	raw, err := ReadGodzillaPayload("payload_test.dll")
	if err != nil {
		panic(fmt.Sprintf("restore csharp payload failed: %v", err))
	}
	return raw
}()

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

// EncryptPayload 加密payload
var EncryptPayload = map[string]map[string]string{}

func GetHexYakPayload(filename string) (string, error) {
	handleFile := func(filename string) string {
		fileinfo := strings.Split(filename, ".")
		if len(fileinfo) != 2 {
			panic("filename analyze fails, filename cannot split filename and ext")
		}
		filename = fileinfo[0]
		switch fileinfo[1] {
		case ypb.ShellScript_PHP.String(), strings.ToLower(ypb.ShellScript_PHP.String()):
			return filename + ".php"
		case ypb.ShellScript_JSP.String(), strings.ToLower(ypb.ShellScript_JSP.String()):
			return filename + ".class"
		case ypb.ShellScript_ASPX.String(), strings.ToLower(ypb.ShellScript_ASPX.String()):
			return filename + ".dll"
		default:
			panic("file ext not match")
		}
	}

	file, err := FS.ReadFile(fmt.Sprintf("%s/%s.txt", yakshellStaticDir, handleFile(filename)))
	if err != nil {
		return "", err
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
	bytes, err := DecryptFunc(file)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func init() {
	dirs, err := FS.ReadDir(behinderStaticDir)
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
		raw, err := FS.ReadFile(behinderStaticDir + "/" + i.Name())
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

	//将加密方式加入
	dir, err := FS.ReadDir(yakshellEncryptDir)
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
		file, err := FS.ReadFile(yakshellEncryptDir + "/" + entry.Name())
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
