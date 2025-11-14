package ssaapi

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func (c *Config) parseFSFromInfo(raw string) (fi.FileSystem, error) {
	if raw == "" {
		return nil, utils.Errorf("info is empty ")
	}

	codeSource, err := ssaconfig.New(ssaconfig.ModeCodeSource, ssaconfig.WithCodeSourceJson(raw))
	if err != nil {
		return nil, utils.Errorf("failed to new code source: %v", err)
	}

	c.Processf(0, "parse info: %s", codeSource.GetCodeSourceKind())
	defer func() {
		c.Processf(0, "parse info finish")
	}()

	var baseFS fi.FileSystem
	switch codeSource.GetCodeSourceKind() {
	case ssaconfig.CodeSourceLocal:
		baseFS = filesys.NewRelLocalFs(codeSource.GetCodeSourceLocalFile())
	case ssaconfig.CodeSourceCompression:
		baseFS, err = getZipFile(codeSource)
		if err != nil {
			return nil, err
		}
	case ssaconfig.CodeSourceJar:
		zipfs, err := getZipFile(codeSource)
		if err != nil {
			return nil, utils.Errorf("jar file error: %v", err)
		}
		baseFS = filesys.NewUnifiedFS(javaclassparser.NewJarFS(zipfs),
			filesys.WithUnifiedFsExtMap(".class", ".java"),
		)
	case ssaconfig.CodeSourceGit:
		baseFS, err = gitFs(codeSource, c.Processf)
		if err != nil {
			return nil, err
		}
	case ssaconfig.CodeSourceSvn:
		return svnFs(codeSource)
	default:
		return nil, utils.Errorf("unsupported kind: %s", codeSource.GetCodeSourceKind())
	}

	return baseFS, nil
}

// 已弃用
// func (c *Config) wrapWithPreprocessedFS(fs fi.FileSystem) (fi.FileSystem, error) {
// 	if c.language != consts.C {
// 		return fs, nil
// 	}

// 	c.Processf(0, "wrapping filesystem with C preprocessor support")
// 	preprocessedFS, err := filesys.NewPreprocessedFS(fs)
// 	if err != nil {
// 		log.Warnf("failed to create preprocessed C filesystem: %v, using original", err)
// 		return fs, nil
// 	}
// 	return preprocessedFS, nil
// }

func getZipFile(codeSource *ssaconfig.Config) (*filesys.ZipFS, error) {
	// use local
	if codeSource.GetCodeSourceLocalFile() != "" {
		return filesys.NewZipFSFromLocal(codeSource.GetCodeSourceLocalFile())
	}
	if codeSource.GetCodeSourceURL() == "" {
		return nil, utils.Errorf("url is empty ")
	}
	// download file
	resp, _, err := poc.DoGET(codeSource.GetCodeSourceURL())
	if err != nil {
		return nil, err
	}
	if resp.GetStatusCode() != 200 {
		return nil, utils.Errorf("download file error: %v", resp.GetStatusCode())
	}

	bytes.NewReader(resp.GetBody())
	return filesys.NewZipFSRaw(bytes.NewReader(resp.GetBody()), int64(len(resp.GetBody())))
}

func gitFs(codeSource *ssaconfig.Config, process func(float64, string, ...any)) (fi.FileSystem, error) {
	if codeSource.GetCodeSourceURL() == "" {
		return nil, utils.Errorf("git url is empty ")
	}
	process(0, "start git clone process from %s", codeSource.GetCodeSourceURL())
	local := path.Join(os.TempDir(), fmt.Sprintf("%s-%s", "yakgit", utils.RandStringBytes(8)))
	// create template director
	if err := os.MkdirAll(local, 0755); err != nil {
		return nil, utils.Errorf("create temp dir error: %v", err)
	}
	log.Info("local : ", local)

	opts := make([]yakgit.Option, 0)
	opts = append(opts, yakgit.WithBranch(codeSource.GetCodeSourceBranch()))
	if proxyURL := codeSource.GetCodeSourceProxyURL(); proxyURL != "" {
		proxyUser, proxyPassword := codeSource.GetCodeSourceProxyAuth()
		opts = append(opts, yakgit.WithProxy(proxyURL, proxyUser, proxyPassword))
	}
	if opt := parseAuth(codeSource.GetCodeSourceAuth()); opt != nil {
		opts = append(opts, opt)
	}
	opts = append(opts, yakgit.WithHTTPOptions(poc.WithRetryTimes(10)))
	if err := yakgit.Clone(codeSource.GetCodeSourceURL(), local, opts...); err != nil {
		return nil, err
	}
	targetPath := filepath.Join(local, codeSource.GetCodeSourcePath())
	_, err := os.Stat(targetPath)
	if err != nil {
		log.Errorf("not found this path,start compile local path")
		targetPath = local
	}
	process(0, "git clone finish start compile...")
	return filesys.NewRelLocalFs(targetPath), nil
}

func parseAuth(auth *ssaconfig.AuthConfigInfo) yakgit.Option {
	if auth == nil {
		return nil
	}
	switch auth.Kind {
	case "password":
		return yakgit.WithUsernamePassword(auth.UserName, auth.Password)
	case "ssh_key":
		return yakgit.WithPrivateKey(auth.UserName, auth.KeyPath, auth.Password)
	}
	return nil
}

func svnFs(codeSource *ssaconfig.Config) (fi.FileSystem, error) {
	return nil, utils.Errorf("unimplemented ")
}
