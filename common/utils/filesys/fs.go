package filesys

import (
	"errors"
	"strings"

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

var SkipDir = errors.New("skip dir")
var SkipAll = errors.New("skip all")

func recursive(raw string, c Config, opts ...Option) (retErr error) {
	c.dirMatch = nil
	for _, opt := range opts {
		opt(&c)
	}
	if c.fileSystem == nil {
		return utils.Errorf("file system is nil")
	}

	var fileCount int64
	var dirCount int64
	var totalCount int64

	var walkSingleFile func(string) error
	var walkDir func(path string) error

	walkSingleFile = func(path string) error {
		info, err := c.fileSystem.Stat(path)
		if err != nil {
			return nil // 忽略单个文件的错误，继续处理其他文件
		}

		// 预先检查是否会超出限制
		if info.IsDir() {
			if c.dirLimit > 0 && dirCount >= c.dirLimit {
				return SkipAll
			}
		} else {
			if c.fileLimit > 0 && fileCount >= c.fileLimit {
				return SkipAll
			}
		}
		if c.totalLimit > 0 && totalCount >= c.totalLimit {
			return SkipAll
		}

		// 增加计数
		totalCount++
		if info.IsDir() {
			dirCount++
		} else {
			fileCount++
		}

		if c.onStat != nil {
			if err := c.onStat(info.IsDir(), path, info); err != nil {
				if err == SkipDir {
					return nil
				}
				if err == SkipAll {
					return err
				}
				return err
			}
		}

		if info.IsDir() {
			if c.onDirStat != nil {
				if err := c.onDirStat(path, info); err != nil {
					if err == SkipDir {
						return nil
					}
					if err == SkipAll {
						return err
					}
					return err
				}
			}

			for _, dirOpt := range c.dirMatch {
				relPath := strings.TrimPrefix(path, raw+string(c.fileSystem.GetSeparators()))
				if dirOpt.inst.Match(relPath) {
					if err := recursive(path, c, dirOpt.opts...); err != nil {
						if err == SkipAll {
							return err
						}
						return err
					}
					return nil
				}
			}

			if c.RecursiveDirectory {
				if err := walkDir(path); err != nil {
					if err == SkipAll {
						return err
					}
					return err
				}
			}
		} else {
			if c.onFileStat != nil {
				if err := c.onFileStat(path, info); err != nil {
					if err == SkipAll {
						return err
					}
					return err
				}
			}
		}
		return nil
	}

	walkDir = func(path string) error {
		dirs, err := c.fileSystem.ReadDir(path)
		if err != nil {
			return nil // 忽略单个目录的错误，继续处理其他目录
		}

		for _, d := range dirs {
			if c.isStop() {
				return nil
			}

			targetFile := c.fileSystem.Join(path, d.Name())
			if err := walkSingleFile(targetFile); err != nil {
				if err == SkipAll {
					return err // 达到限制时直接返回
				}
				// 其他错误继续处理
			}
		}

		if c.onDirWalkEnd != nil {
			if err := c.onDirWalkEnd(path); err != nil {
				if err == SkipAll {
					return err
				}
				return err
			}
		}
		return nil
	}

	info, err := c.fileSystem.Stat(raw)
	if err != nil {
		return utils.Errorf("stat %s failed: %v", raw, err)
	}
	if !info.IsDir() {
		return utils.Errorf("root path is not a directory: %s", raw)
	}

	if c.onStart != nil {
		if err := c.onStart(raw, info.IsDir()); err != nil {
			if err == SkipAll {
				return nil
			}
			return err
		}
	}

	if c.RecursiveDirectory {
		if err := walkDir(raw); err != nil {
			if err == SkipAll {
				return nil
			}
			return err
		}
	} else {
		if err := walkSingleFile(raw); err != nil {
			if err == SkipAll {
				return nil
			}
			return err
		}
	}
	return nil
}
