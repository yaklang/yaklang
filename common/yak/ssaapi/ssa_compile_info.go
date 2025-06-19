package ssaapi

import (
	"bytes"
	"encoding/json"
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
)

type ConfigInfoKind string

const (
	Local       ConfigInfoKind = "local"
	Compression ConfigInfoKind = "compression"
	Jar         ConfigInfoKind = "jar"
	Git         ConfigInfoKind = "git"
	Svn         ConfigInfoKind = "svn"
)

type ConfigInfo struct {
	Kind ConfigInfoKind `json:"kind"`
	// The kind of the parse: "local", "compression"

	/*
		"local":
			* "local_file":  path to the local directory
		"compression":
			* "local_file":  path to the local compressed file
		"jar":
			"local_file":  path to the local jar file
		"git":
			* "git_url":  git url
			"git_branch":  git branch
			auth
			proxy
		"svn":
			* "svn_url":  svn url
			"svn_branch":  svn branch
			auth
			proxy
	*/

	LocalFile string `json:"local_file"`
	URL       string `json:"url"` //  for git/svn/tar/jar
	// git or svn
	Branch  string `json:"branch"`
	GitPath string `json:"path"`
	Auth    *auth  `json:"ce"`
	Proxy   *Proxy `json:"proxy"`
}

func (c ConfigInfo) String() string {
	b, _ := json.Marshal(c)
	return string(b)
}

type auth struct {
	Kind string `json:"kind"`
	/*
		"password":
			password // password or token
			username
		"ssh_key":
			*key_path // private key path
			user_name
			password
	*/
	UserName string `json:"user_name"`
	Password string `json:"password"`
	KeyPath  string `json:"key_path"`
}

type Proxy struct {
	URL      string `json:"url"` // * require
	User     string `json:"user"`
	PassWord string `json:"password"`
}

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
	case Local:
		return filesys.NewRelLocalFs(info.LocalFile), nil
	case Compression:
		return getZipFile(&info)
	case Jar:
		zipfs, err := getZipFile(&info)
		if err != nil {
			return nil, utils.Errorf("jar file error: %v", err)
		}
		fs := filesys.NewUnifiedFS(javaclassparser.NewJarFS(zipfs),
			filesys.WithUnifiedFsExtMap(".class", ".java"),
		)
		return fs, nil
	case Git:
		return gitFs(&info, c.Processf)
	case Svn:
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
		opts = append(opts, yakgit.WithProxy(proxy.URL, proxy.User, proxy.PassWord))
	}
	if opt := parseAuth(info.Auth); opt != nil {
		opts = append(opts, opt)
	}
	opts = append(opts, yakgit.WithHTTPOptions(poc.WithRetryTimes(10)))
	if err := yakgit.Clone(info.URL, local, opts...); err != nil {
		return nil, err
	}
	targetPath := filepath.Join(local, info.GitPath)
	_, err := os.Stat(targetPath)
	if err != nil {
		log.Errorf("not found this path,start compile local path")
		targetPath = local
	}
	process(0, "git clone finish start compile...")
	return filesys.NewRelLocalFs(targetPath), nil
}

func parseAuth(auth *auth) yakgit.Option {
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
