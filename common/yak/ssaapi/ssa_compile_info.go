package ssaapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/yakgit"
)

type ConfigInfo = schema.CodeSourceInfo

func (c *config) parseFSFromInfo(raw string) (fi.FileSystem, error) {
	if raw == "" {
		return nil, utils.Errorf("info is empty ")
	}
	info := ConfigInfo{}
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return nil, utils.Errorf("error unmarshal info: %v", err)
	}
	c.Processf(0, "parse info: %s", info.Kind)
	defer func() {
		c.Processf(0, "parse info finish")
	}()
	switch info.Kind {
	case schema.CodeSourceLocal:
		return filesys.NewRelLocalFs(info.LocalFile), nil
	case schema.CodeSourceCompression:
		return getZipFile(&info)
	case schema.CodeSourceJar:
		zipfs, err := getZipFile(&info)
		if err != nil {
			return nil, utils.Errorf("jar file error: %v", err)
		}
		fs := filesys.NewUnifiedFS(javaclassparser.NewJarFS(zipfs),
			filesys.WithUnifiedFsExtMap(".class", ".java"),
		)
		return fs, nil
	case schema.CodeSourceGit:
		return gitFs(&info, c.Processf)
	case schema.CodeSourceSvn:
		return svnFs(&info)
	}
	return nil, utils.Errorf("unsupported kind: %s", info.Kind)
}

func getZipFile(info *ConfigInfo) (*filesys.ZipFS, error) {
	// use local
	if info.LocalFile != "" {
		return filesys.NewZipFSFromLocal(info.LocalFile)
	}
	if info.URL == "" {
		return nil, utils.Errorf("url is empty ")
	}
	// download file
	resp, _, err := poc.DoGET(info.URL)
	if err != nil {
		return nil, err
	}
	if resp.GetStatusCode() != 200 {
		return nil, utils.Errorf("download file error: %v", resp.GetStatusCode())
	}

	bytes.NewReader(resp.GetBody())
	return filesys.NewZipFSRaw(bytes.NewReader(resp.GetBody()), int64(len(resp.GetBody())))
}

func gitFs(info *ConfigInfo, process func(float64, string, ...any)) (fi.FileSystem, error) {
	if info.URL == "" {
		return nil, utils.Errorf("git url is empty ")
	}
	process(0, "start git clone process from %s", info.URL)
	local := path.Join(os.TempDir(), fmt.Sprintf("%s-%s", "yakgit", utils.RandStringBytes(8)))
	// create template director
	if err := os.MkdirAll(local, 0755); err != nil {
		return nil, utils.Errorf("create temp dir error: %v", err)
	}
	log.Info("local : ", local)

	opts := make([]yakgit.Option, 0)
	opts = append(opts, yakgit.WithBranch(info.Branch))
	if proxy := info.Proxy; proxy != nil && proxy.URL != "" {
		opts = append(opts, yakgit.WithProxy(proxy.URL, proxy.User, proxy.Password))
	}
	if opt := parseAuth(info.Auth); opt != nil {
		opts = append(opts, opt)
	}
	opts = append(opts, yakgit.WithHTTPOptions(poc.WithRetryTimes(10)))
	if err := yakgit.Clone(info.URL, local, opts...); err != nil {
		return nil, err
	}
	targetPath := filepath.Join(local, info.Path)
	_, err := os.Stat(targetPath)
	if err != nil {
		log.Errorf("not found this path,start compile local path")
		targetPath = local
	}
	process(0, "git clone finish start compile...")
	return filesys.NewRelLocalFs(targetPath), nil
}

func parseAuth(auth *schema.AuthConfigInfo) yakgit.Option {
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

func svnFs(info *ConfigInfo) (fi.FileSystem, error) {
	return nil, utils.Errorf("unimplemented ")
}
