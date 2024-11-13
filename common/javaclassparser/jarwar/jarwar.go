package jarwar

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

type JarWar struct {
	// License 存储 JAR/WAR 文件中的许可证信息
	// 通常从 META-INF/LICENSE 文件中读取
	License    string
	ManifestMF JarManifest
	fs         *javaclassparser.FS

	// 存储反编译失败的文件列表
	failedDecompiledFiles map[string]struct{}
	failedFilesLock       sync.RWMutex
}

func NewFromJarFS(fs *javaclassparser.FS) (*JarWar, error) {
	// Check jar or war
	if entries, err := fs.ReadDir("WEB-INF/"); err == nil && len(entries) > 0 {
		return &JarWar{
			fs:                    fs,
			failedDecompiledFiles: make(map[string]struct{}),
		}, nil
	} else if manifest, err := fs.ZipFS.ReadFile("META-INF/MANIFEST.MF"); err == nil && len(manifest) > 0 {
		if strings.Contains(string(manifest), "Manifest-Version") {
			result := &JarWar{
				ManifestMF:            ParseJarManifest(string(manifest)),
				fs:                    fs,
				failedDecompiledFiles: make(map[string]struct{}),
			}
			if license, err := fs.ZipFS.ReadFile("META-INF/LICENSE"); err == nil {
				result.License = string(license)
			}

			return result, nil
		}
	}
	var msg string
	if !utils.IsNil(fs) {
		msg = "\n" + filesys.DumpTreeView(fs)
	}
	return nil, fmt.Errorf("unknown file type: (struct)%v", msg)
}

func New(compressedFile string) (*JarWar, error) {
	if utils.GetFirstExistedFile(compressedFile) == "" {
		return nil, fmt.Errorf("file not existed: %s", compressedFile)
	}
	fs, err := javaclassparser.NewJarFSFromLocal(compressedFile)
	if err != nil {
		return nil, utils.Wrap(err, "failed to create jar fs")
	}
	fsIns, err := NewFromJarFS(fs)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create jarwar")
	}
	return fsIns, nil
}

func (j *JarWar) GetStructDump() string {
	return filesys.DumpTreeView(j.fs)
}

func (j *JarWar) DumpToLocalFileSystem(dir string) error {
	if utils.GetFirstExistedPath(dir) == "" {
		log.Info("output directory not existed, create it, os.MkdirAll")
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return utils.Wrap(err, "os.MkdirAll failed")
		}
	}

	err := filesys.Recursive(".", filesys.WithFileSystem(j.fs), filesys.WithStat(func(isDir bool, s string, info fs.FileInfo) error {
		target := filepath.Join(dir, s)
		if isDir {
			log.Infof("create dir: %v", target)
			err := os.MkdirAll(target, 0755)
			if err != nil {
				log.Warnf("os.MkdirAll failed: %v", err)
				return err
			}
			return nil
		}

		if filepath.Ext(s) == ".class" {
			// 尝试反编译.class文件
			decompiled, err := j.fs.ReadFile(s)
			if err != nil {
				log.Warnf("Decompile failed, keep original(%v): %v", s, err)
				raw, err := j.fs.ZipFS.ReadFile(s)
				if err != nil {
					log.Warnf("ReadFile failed: %v", err)
					return utils.Wrap(err, "ReadFile failed during decompilation")
				}
				// 保存反编译失败的文件(带锁去重)
				j.failedFilesLock.Lock()
				j.failedDecompiledFiles[s] = struct{}{}
				j.failedFilesLock.Unlock()
				return os.WriteFile(target, raw, 0755)
			}
			// 将.class文件改为.java后缀
			javaTarget := strings.TrimSuffix(target, ".class") + ".java"
			return os.WriteFile(javaTarget, decompiled, 0755)
		} else {
			// 非.class文件，保持原样
			raw, err := j.fs.ZipFS.ReadFile(s)
			if err != nil {
				log.Warnf("ReadFile failed: %v", err)
				return err
			}
			return os.WriteFile(target, raw, 0755)
		}
	}))

	if err != nil {
		return utils.Wrap(err, "Recursive file system dump failed")
	}
	return nil
}
