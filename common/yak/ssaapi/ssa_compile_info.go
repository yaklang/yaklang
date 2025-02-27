package ssaapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/yakgit"
)

type config_info struct {
	Kind string `json:"kind"`
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
	Branch string `json:"branch"`
	Auth   *auth  `json:"ce"`
	Proxy  *proxy `json:"proxy"`
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

type proxy struct {
	URL      string `json:"url"` // * require
	User     string `json:"user"`
	PassWord string `json:"password"`
}

func (c *config) parseFSFromInfo(raw string) (fi.FileSystem, error) {
	if raw == "" {
		return nil, utils.Errorf("info is empty ")
	}
	info := config_info{}
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return nil, utils.Errorf("error unmarshal info: %v", err)
	}
	c.Processf(0, "start parse info: %s", info.Kind)
	switch info.Kind {
	case "local":
		return filesys.NewRelLocalFs(info.LocalFile), nil
	case "compression":
		return getZipFile(&info)
	case "jar":
		zipfs, err := getZipFile(&info)
		if err != nil {
			return nil, utils.Errorf("jar file error: %v", err)
		}
		fs := filesys.NewUnifiedFS(javaclassparser.NewJarFS(zipfs),
			filesys.WithUnifiedFsExtMap(".class", ".java"),
		)
		return fs, nil
	case "git":
		return gitFs(&info, c.Processf)
	case "svn":
		return svnFs(&info)
	}
	return nil, utils.Errorf("unsupported kind: %s", info.Kind)
}

func (info config_info) String() string {
	b, _ := json.Marshal(info)
	return string(b)
}

func getZipFile(info *config_info) (*filesys.ZipFS, error) {
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

func gitFs(info *config_info, process func(float64, string, ...any)) (fi.FileSystem, error) {
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
	if err := yakgit.Clone(info.URL, local, opts...); err != nil {
		return nil, err
	}
	process(0, "git clone finish start compile...")
	return filesys.NewRelLocalFs(local), nil
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

func svnFs(info *config_info) (fi.FileSystem, error) {
	return nil, utils.Errorf("unimplemented ")
}
