package ssaapi

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/javajive/classparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
)

var (
	cloneSSAGitRepository = yakgit.Clone
	// ssaGitCloneSleep is replaced in tests to avoid real backoff delays.
	ssaGitCloneSleep = time.Sleep
)

func (c *Config) parseFSFromInfo() (fi.FileSystem, error) {
	c.Processf(0, "parse info: %s", c.GetCodeSourceKind())
	defer func() {
		c.Processf(0, "parse info finish")
	}()

	var baseFS fi.FileSystem
	var err error
	switch c.GetCodeSourceKind() {
	case ssaconfig.CodeSourceLocal:
		baseFS = filesys.NewRelLocalFs(c.GetCodeSourceLocalFile())
	case ssaconfig.CodeSourceCompression:
		baseFS, err = getZipFile(c)
		if err != nil {
			return nil, err
		}
	case ssaconfig.CodeSourceJar:
		jarFS, err := getJarFS(c)
		if err != nil {
			return nil, utils.Errorf("jar file error: %v", err)
		}
		unifiedFS := filesys.NewUnifiedFS(jarFS,
			filesys.WithUnifiedFsExtMap(".class", ".java"),
		)
		baseFS = unifiedFS
	case ssaconfig.CodeSourceGit:
		baseFS, err = gitFs(c)
		if err != nil {
			return nil, err
		}
	case ssaconfig.CodeSourceSvn:
		return svnFs(c)
	case ssaconfig.CodeSourceNone:
		return nil, nil
	default:
		return nil, utils.Errorf("unsupported kind: %s", c.GetCodeSourceKind())
	}

	return baseFS, nil
}

func getZipFile(codeSource *Config) (*filesys.ZipFS, error) {
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

// getJarFS creates a javaclassparser.JarFS from either a local file or a
// downloaded URL.
func getJarFS(c *Config) (*javaclassparser.JarFS, error) {
	// use local
	if c.GetCodeSourceLocalFile() != "" {
		return javaclassparser.NewJarFSFromLocalWithOptions(c.GetCodeSourceLocalFile(), c.GetCodeSourceJarRecursiveParse())
	}
	if c.GetCodeSourceURL() == "" {
		return nil, utils.Errorf("url is empty ")
	}
	// download file
	resp, _, err := poc.DoGET(c.GetCodeSourceURL())
	if err != nil {
		return nil, err
	}
	if resp.GetStatusCode() != 200 {
		return nil, utils.Errorf("download file error: %v", resp.GetStatusCode())
	}
	// Write downloaded bytes to a temp file so we can use NewJarFSFromLocal
	tmpFile, err := os.CreateTemp("", "yaklang-jar-*.jar")
	if err != nil {
		return nil, utils.Errorf("create temp file error: %v", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := tmpFile.Write(resp.GetBody()); err != nil {
		tmpFile.Close()
		return nil, utils.Errorf("write temp file error: %v", err)
	}
	tmpFile.Close()
	return javaclassparser.NewJarFSFromLocalWithOptions(tmpPath, c.GetCodeSourceJarRecursiveParse())
}

func gitFs(codeSource *Config) (fi.FileSystem, error) {
	process := codeSource.Processf
	if codeSource.GetCodeSourceURL() == "" {
		return nil, utils.Errorf("git url is empty ")
	}
	process(0, "start git clone process from %s", codeSource.GetCodeSourceURL())
	local, cleanup, err := ssagitworkdir.Prepare(codeSource.GetContext(), os.Getpid())
	if err != nil {
		return nil, err
	}
	cleanupOnError := true
	cleanupWorkspace := func() {
		if err := cleanup(); err != nil {
			log.Errorf("failed to cleanup git workspace %s: %v", local, err)
		}
	}
	defer func() {
		if cleanupOnError {
			cleanupWorkspace()
		}
	}()
	log.Info("git clone workspace: ", local)

	opts := make([]yakgit.Option, 0)
	// Scan compiles only need the tip of the configured branch. A full clone
	// downloads every ref/history and is the main reason large git sources
	// feel slow; mirror `git clone --depth 1 --single-branch --no-tags -b <branch>`.
	opts = append(opts,
		yakgit.WithDepth(1),
		yakgit.WithSingleBranch(true),
		yakgit.WithNoFetchTags(true),
		yakgit.WithRecuriveSubmodule(false),
	)
	if branch := strings.TrimSpace(codeSource.GetCodeSourceBranch()); branch != "" {
		opts = append(opts, yakgit.WithBranch(branch))
	}
	if proxyURL := codeSource.GetCodeSourceProxyURL(); proxyURL != "" {
		proxyUser, proxyPassword := codeSource.GetCodeSourceProxyAuth()
		opts = append(opts, yakgit.WithProxy(proxyURL, proxyUser, proxyPassword))
	}
	authOpts := parseAuth(codeSource.GetCodeSourceAuth())
	opts = append(opts, authOpts...)
	opts = append(opts, yakgit.WithContext(codeSource.GetContext()))
	opts = append(opts, yakgit.WithHTTPOptions(poc.WithRetryTimes(10)))
	if err := cloneSSAGitRepositoryWithRetry(
		codeSource.GetCodeSourceURL(),
		local,
		opts,
		func(format string, args ...any) {
			process(0, format, args...)
		},
	); err != nil {
		return nil, ssagitworkdir.WrapCloneError(codeSource.GetContext(), local, err)
	}

	// DefaultConfig can fail after the clone but before ParseProject installs its
	// defer. Register ownership only after the clone succeeds; the local defer
	// above owns every earlier failure.
	codeSource.AddCleanupFunc(func() {
		log.Infof("cleaning up git workspace: %s", local)
		cleanupWorkspace()
	})
	cleanupOnError = false
	targetPath := filepath.Join(local, codeSource.GetCodeSourcePath())
	_, err = os.Stat(targetPath)
	if err != nil {
		log.Errorf("not found this path,start compile local path")
		targetPath = local
	}
	process(0, "git clone finish start compile...")
	return filesys.NewRelLocalFs(targetPath), nil
}

func parseAuth(auth *ssaconfig.AuthConfigInfo) []yakgit.Option {
	if auth == nil {
		return nil
	}

	var opts []yakgit.Option

	switch auth.Kind {
	case "password", "token":
		opts = append(opts, yakgit.WithUsernamePassword(auth.UserName, auth.Password))
	case "ssh_key":
		if auth.KeyContent != "" {
			opts = append(opts, yakgit.WithPrivateKeyContent(auth.UserName, auth.KeyContent, auth.Password))
		} else if auth.KeyPath != "" {
			opts = append(opts, yakgit.WithPrivateKey(auth.UserName, auth.KeyPath, auth.Password))
		}
		opts = append(opts, yakgit.WithInsecureIgnoreHostKey())
	}

	return opts
}

func svnFs(codeSource *Config) (fi.FileSystem, error) {
	return nil, utils.Errorf("unimplemented ")
}
