package ssaconfig

// import (
// 	"bytes"
// 	"encoding/json"
// 	"fmt"
// 	"os"
// 	"path"
// 	"path/filepath"

// 	"github.com/yaklang/yaklang/common/javaclassparser"
// 	"github.com/yaklang/yaklang/common/log"
// 	"github.com/yaklang/yaklang/common/utils"
// 	"github.com/yaklang/yaklang/common/utils/filesys"
// 	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
// 	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
// 	"github.com/yaklang/yaklang/common/utils/yakgit"
// )

// func parseFSFromInfo(raw string) (fi.FileSystem, error) {
// 	if raw == "" {
// 		return nil, utils.Errorf("info is empty ")
// 	}

// 	var codeSource = &CodeSourceInfo{}
// 	err := json.Unmarshal([]byte(raw), &codeSource)
// 	if err != nil {
// 		return nil, utils.Errorf("parse code source info error: %v", err)
// 	}

// 	var baseFS fi.FileSystem
// 	switch codeSource.Kind {
// 	case CodeSourceLocal:
// 		baseFS = filesys.NewRelLocalFs(codeSource.LocalFile)
// 	case CodeSourceCompression:
// 		baseFS, err = getZipFile(codeSource)
// 		if err != nil {
// 			return nil, err
// 		}
// 	case CodeSourceJar:
// 		zipfs, err := getZipFile(codeSource)
// 		if err != nil {
// 			return nil, utils.Errorf("jar file error: %v", err)
// 		}
// 		baseFS = filesys.NewUnifiedFS(javaclassparser.NewJarFS(zipfs),
// 			filesys.WithUnifiedFsExtMap(".class", ".java"),
// 		)
// 	case CodeSourceGit:
// 		baseFS, err = gitFs(codeSource)
// 		if err != nil {
// 			return nil, err
// 		}
// 	case CodeSourceSvn:
// 		return svnFs(codeSource)
// 	default:
// 		return nil, utils.Errorf("unsupported kind: %s", codeSource.Kind)
// 	}

// 	return baseFS, nil
// }

// func getZipFile(codeSource *CodeSourceInfo) (*filesys.ZipFS, error) {
// 	// use local
// 	if codeSource.LocalFile != "" {
// 		return filesys.NewZipFSFromLocal(codeSource.LocalFile)
// 	}
// 	if codeSource.URL == "" {
// 		return nil, utils.Errorf("url is empty ")
// 	}
// 	// download file
// 	resp, _, err := poc.DoGET(codeSource.URL)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if resp.GetStatusCode() != 200 {
// 		return nil, utils.Errorf("download file error: %v", resp.GetStatusCode())
// 	}

// 	bytes.NewReader(resp.GetBody())
// 	return filesys.NewZipFSRaw(bytes.NewReader(resp.GetBody()), int64(len(resp.GetBody())))
// }

// func gitFs(codeSource *CodeSourceInfo) (fi.FileSystem, error) {
// 	if codeSource.URL == "" {
// 		return nil, utils.Errorf("git url is empty ")
// 	}
// 	local := path.Join(os.TempDir(), fmt.Sprintf("%s-%s", "yakgit", utils.RandStringBytes(8)))
// 	// create template director
// 	if err := os.MkdirAll(local, 0755); err != nil {
// 		return nil, utils.Errorf("create temp dir error: %v", err)
// 	}
// 	log.Info("local : ", local)

// 	opts := make([]yakgit.Option, 0)
// 	opts = append(opts, yakgit.WithBranch(codeSource.Branch))
// 	if proxyURL := codeSource.Proxy.URL; proxyURL != "" {
// 		proxyUser, proxyPassword := codeSource.Proxy.User, codeSource.Proxy.Password
// 		opts = append(opts, yakgit.WithProxy(proxyURL, proxyUser, proxyPassword))
// 	}
// 	if opt := parseAuth(codeSource.Auth); opt != nil {
// 		opts = append(opts, opt)
// 	}
// 	opts = append(opts, yakgit.WithHTTPOptions(poc.WithRetryTimes(10)))
// 	if err := yakgit.Clone(codeSource.URL, local, opts...); err != nil {
// 		return nil, err
// 	}
// 	targetPath := filepath.Join(local, codeSource.Path)
// 	_, err := os.Stat(targetPath)
// 	if err != nil {
// 		log.Errorf("not found this path,start compile local path")
// 		targetPath = local
// 	}
// 	return filesys.NewRelLocalFs(targetPath), nil
// }

// func parseAuth(auth *AuthConfigInfo) yakgit.Option {
// 	if auth == nil {
// 		return nil
// 	}
// 	switch auth.Kind {
// 	case "password":
// 		return yakgit.WithUsernamePassword(auth.UserName, auth.Password)
// 	case "ssh_key":
// 		return yakgit.WithPrivateKey(auth.UserName, auth.KeyPath, auth.Password)
// 	}
// 	return nil
// }

// func svnFs(codeSource *CodeSourceInfo) (fi.FileSystem, error) {
// 	return nil, utils.Errorf("unimplemented ")
// }
