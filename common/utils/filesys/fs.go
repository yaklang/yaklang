package filesys

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/utils"
)

func SimpleRecursive(opts ...Option) error {
	c := NewConfig()
	for _, opt := range opts {
		opt(c)
	}
	start := ""
	for _, entryPath := range []string{
		".", "", "/",
	} {
		entries, _ := c.fileSystem.ReadDir(entryPath)
		if len(entries) > 0 {
			start = entryPath
			break
		}
	}
	if start == "" {
		return utils.Error("no entry found")
	}
	return recursive(start, *c, opts...)
}

// Recursive recursively walk through the file system
// raw: the root path
// opts: options
// return: error
//
// Example:
// ```
// err := filesys.Recursive( //
//
//	"testdata",
//	filesys.dir(["cc", "dd"], filesys.onFileStat((name, info) => {})),
//
// )
// ```
func Recursive(raw string, opts ...Option) error {
	c := NewConfig()
	return recursive(raw, *c, opts...)
}

func glance(i filesys_interface.FileSystem) string {
	var buf bytes.Buffer
	var fileCount = 0
	var dirCount = 0

	overSize := false
	ctx, cancel := context.WithCancel(context.Background())
	err := Recursive(".",
		WithContext(ctx),
		WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
			if isDir {
				dirCount++
			} else {
				fileCount++
			}
			if dirCount+fileCount > 5000 {
				overSize = true
				cancel()
				return utils.Wrap(SkipAll, "too many files, stop counting")
			}
			return nil
		}), WithFileSystem(i))
	_ = err // ignore error

	// 获取当前工作目录的绝对路径
	currentDir := "."
	if wd, err := i.Getwd(); err == nil && wd != "" {
		currentDir = wd
	}

	// 如果是本地文件系统类型，并且返回的是相对路径，尝试获取绝对路径
	if currentDir == "." || !filepath.IsAbs(currentDir) {
		switch i.(type) {
		case *LocalFs:
			if absPath, err := os.Getwd(); err == nil {
				currentDir = absPath
			}
		case *RelLocalFs:
			// 对于 RelLocalFs，将相对路径转换为绝对路径
			if absPath, err := filepath.Abs(currentDir); err == nil {
				currentDir = absPath
			}
		default:
			// 对于其他类型，如果不是绝对路径，尝试转换
			if !filepath.IsAbs(currentDir) {
				if absPath, err := filepath.Abs(currentDir); err == nil {
					currentDir = absPath
				}
			}
		}
	}

	// 写入当前目录和统计信息
	buf.WriteString(fmt.Sprintf("current dir: %s\n", currentDir))
	buf.WriteString(fmt.Sprintf("total: %v[dir: %v file: %v]", fileCount+dirCount, dirCount, fileCount))
	if overSize {
		buf.WriteString(" (too many files, over 5000 files, limited show)\n")
	} else {
		buf.WriteString("\n")
	}

	log.Infof("start to dump tree view with limits for fs glance: depth: %v, lines: %v", 4, 50)
	// 使用 DumpTreeViewWithLimits 直接从 FileSystem 生成树形视图，限制为 4 层深度，50 行输出
	treeOutput := DumpTreeViewWithLimits(i, 4, 50)
	if treeOutput != "" {
		buf.WriteString(treeOutput)
	}

	if err != nil {
		buf.WriteString("\nWARN:" + err.Error())
	}
	return buf.String()
}

// Glance is for quickly viewing the basic info in fs
func Glance(localfile any) string {
	switch ret := localfile.(type) {
	case filesys_interface.FileSystem:
		return glance(ret)
	}
	return glance(NewRelLocalFs(utils.InterfaceToString(localfile)))
}

var SkipDir = errors.New("skip dir")
var SkipAll = errors.New("skip all")

func recursive(raw string, c Config, opts ...Option) (retErr error) {
	if c.isStop() {
		return nil
	}

	c.dirMatch = nil
	for _, opt := range opts {
		opt(&c)
	}
	if c.fileSystem == nil {
		return utils.Errorf("file system is nil")
	}

	var lastErr error // if stop return last error

	var fileCount int64
	var dirCount int64
	var totalCount int64

	var walkSingleFile func(string) error
	var walkDir func(path string) error

	walkSingleFile = func(path string) error {
		if c.isStop() {
			return nil
		}
		info, err := c.fileSystem.Stat(path)
		if err != nil {
			return utils.Errorf("stat %s failed: %v", path, err)
		}

		// count
		totalCount++
		if c.totalLimit > 0 && c.totalLimit < totalCount {
			c.Stop()
			log.Warnf("total count limit exceeded: %d", c.totalLimit)
			return SkipAll
		}

		if c.onStat != nil {
			if err := c.onStat(info.IsDir(), path, info); err != nil {
				if err == SkipDir || err == SkipAll {
					return nil
				}
				return err
			}
		}

		if info.IsDir() {
			// dir
			// dir count
			dirCount++
			if c.dirLimit > 0 && c.dirLimit < dirCount {
				c.Stop()
				log.Warnf("dir count limit exceeded: %d", c.dirLimit)
				return SkipAll
			}

			// file stat
			if c.onDirStat != nil {
				if err := c.onDirStat(path, info); err != nil {
					if errors.Is(err, SkipAll) {
						c.Stop()
						return SkipAll
					}
					return err
				}
			}

			for _, dirOpt := range c.dirMatch {
				// if dirOpt.inst == nil {}
				relPath := strings.TrimPrefix(path,
					raw+string(c.fileSystem.GetSeparators()),
				)
				if dirOpt.inst.Match(relPath) {
					return recursive(path, c, dirOpt.opts...)
				}
			}

			if c.RecursiveDirectory {
				err := walkDir(path)
				if err != nil {
					return err
				}
			}

		} else {
			// file
			// file count
			fileCount++
			if c.dirLimit > 0 && c.dirLimit < dirCount {
				return utils.Errorf("dir count limit exceeded: %d", c.dirLimit)
			}

			if fileCount > c.fileLimit {
				c.Stop()
				log.Warnf("file count limit exceeded: %d", c.fileLimit)
				return SkipAll
			}

			if c.onFileStat != nil {
				err = c.onFileStat(path, info)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}

	walkDir = func(path string) error {
		if c.isStop() {
			return nil
		}

		dirs, err := c.fileSystem.ReadDir(path)
		if err != nil {
			return err
		}
		for _, d := range dirs {
			if c.isStop() {
				return lastErr
			}

			targetFile := c.fileSystem.Join(path, d.Name())
			if err := walkSingleFile(targetFile); err != nil {
				lastErr = err
				log.Warnf("walk file %s failed: %v", targetFile, err)
				//return err
			}
			if c.isStop() {
				return lastErr
			}
		}
		if c.onDirWalkEnd != nil {
			if err := c.onDirWalkEnd(path); err != nil {
				return err
			}
		}
		return nil
	}

	base := raw
	info, err := c.fileSystem.Stat(raw)
	if err != nil {
		return utils.Errorf("stat %s failed: %v", raw, err)
	}
	if !info.IsDir() {
		return utils.Errorf("root path is not a directory: %s", raw)
	}

	if c.onStart != nil {
		if err := c.onStart(base, info.IsDir()); err != nil {
			return err
		}
	}

	if err := walkDir(raw); err != nil {
		return err
	}

	return nil
}
