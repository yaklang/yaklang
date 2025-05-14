package consts

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	AuthInfoMutex         = new(sync.Mutex)
	GLOBAL_HTTP_AUTH_INFO []*ypb.AuthInfo
)

func SetGlobalHTTPAuthInfo(info []*ypb.AuthInfo) {
	AuthInfoMutex.Lock()
	defer AuthInfoMutex.Unlock()
	GLOBAL_HTTP_AUTH_INFO = info
}

func GetAuthTypeList(authType string) []string {
	switch strings.ToLower(authType) {
	case "negotiate":
		return []string{"negotiate", "ntlm", "kerberos"}
	default:
		return []string{strings.ToLower(authType)}
	}
}

func GetGlobalHTTPAuthInfo(host, authType string) *ypb.AuthInfo {
	AuthInfoMutex.Lock()
	defer AuthInfoMutex.Unlock()
	anyAuthInfo := new(ypb.AuthInfo)
	gotAnyTypeAuth := false
	for _, info := range GLOBAL_HTTP_AUTH_INFO {
		if !info.Forbidden && utils.HostContains(info.Host, host) {
			if utils.StringSliceContain(GetAuthTypeList(authType), info.AuthType) {
				return info
			}
			if info.AuthType == "any" && !gotAnyTypeAuth { // if got any type auth, save it, just first
				anyAuthInfo = info
				anyAuthInfo.AuthType = authType
				gotAnyTypeAuth = true
			}
		}
	}
	if gotAnyTypeAuth { // if got any type auth, return it
		return anyAuthInfo
	}
	return nil
}

func TempFile(pattern string) (*os.File, error) {
	return ioutil.TempFile(GetDefaultYakitBaseTempDir(), pattern)
}

func TempAIFile(pattern string) (*os.File, error) {
	dirname := filepath.Clean(filepath.Join(GetDefaultYakitBaseTempDir(), "..", "aispace"))
	if os.MkdirAll(dirname, os.ModePerm) != nil {
		dirname = GetDefaultYakitBaseTempDir()
	}
	return ioutil.TempFile(dirname, pattern)
}

func TempAIFileFast(pattern string, datas ...any) string {
	if pattern == "" {
		pattern = "ai-*.tmp"
	}
	f, err := TempAIFile(pattern)
	if err != nil {
		log.Errorf("create temp file error: %v", err)
		return ""
	}
	defer f.Close()
	data := bytes.Join(
		lo.Map(datas, func(item any, _ int) []byte {
			return codec.AnyToBytes(item)
		}),
		[]byte("\r\n"),
	)
	f.Write(data)
	return f.Name()
}

func TempFileFast(datas ...any) string {
	f, err := TempFile("yakit-*.tmp")
	if err != nil {
		log.Errorf("create temp file error: %v", err)
		return ""
	}
	defer f.Close()
	data := bytes.Join(
		lo.Map(datas, func(item any, _ int) []byte {
			return codec.AnyToBytes(item)
		}),
		[]byte("\r\n"),
	)
	f.Write(data)
	return f.Name()
}
func GetFfmpegPath() string {
	defaultPath := GetDefaultYakitProjectsDir()
	var paths []string
	if runtime.GOOS == "darwin" {
		paths = append(paths, filepath.Join(defaultPath, "libs", "ffmpeg"))
		paths = append(paths, filepath.Join(defaultPath, "base", "ffmpeg"))
		paths = append(paths, filepath.Join(defaultPath, "engine", "ffmpeg"))
		paths = append(paths, filepath.Join(defaultPath, "ffmpeg"))
		paths = append(paths, "ffmpeg")
		paths = append(paths, filepath.Join("/", "usr", "local", "bin", "ffmpeg"))
		paths = append(paths, filepath.Join("/", "bin", "ffmpeg"))
		paths = append(paths, filepath.Join("/", "usr", "bin", "ffmpeg"))
	}

	if runtime.GOOS == "windows" {
		paths = append(paths, filepath.Join(defaultPath, "base", "ffmpeg.exe"))
		paths = append(paths, filepath.Join(defaultPath, "libs", "ffmpeg.exe"))
		paths = append(paths, filepath.Join(defaultPath, "engine", "ffmpeg.exe"))
		paths = append(paths, filepath.Join(defaultPath, "ffmpeg.exe"))
		paths = append(paths, "ffmpeg.exe")
	}
	return utils.GetFirstExistedFile(paths...)
}

func GetVulinboxPath() string {
	defaultPath := GetDefaultYakitProjectsDir()
	var paths []string
	if runtime.GOOS == "windows" {
		paths = append(paths, filepath.Join(defaultPath, "base", "vulinbox.exe"))
		paths = append(paths, filepath.Join(defaultPath, "libs", "vulinbox.exe"))
		paths = append(paths, filepath.Join(defaultPath, "engine", "vulinbox.exe"))
		paths = append(paths, filepath.Join(defaultPath, "vulinbox.exe"))
		paths = append(paths, "vulinbox.exe")
	} else {
		paths = append(paths, filepath.Join(defaultPath, "libs", "vulinbox"))
		paths = append(paths, filepath.Join(defaultPath, "base", "vulinbox"))
		paths = append(paths, filepath.Join(defaultPath, "engine", "vulinbox"))
		paths = append(paths, filepath.Join(defaultPath, "vulinbox"))
		paths = append(paths, "vulinbox")
		paths = append(paths, filepath.Join("/", "usr", "local", "bin", "vulinbox"))
		paths = append(paths, filepath.Join("/", "bin", "vulinbox"))
		paths = append(paths, filepath.Join("/", "usr", "bin", "vulinbox"))
	}
	return utils.GetFirstExistedFile(paths...)
}
