package filesys

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
)

func CopyToRefLocal(srcFs filesys_interface.FileSystem, dest string) (*RelLocalFs, error) {
	name := dest // 初始化 name，避免空字符串问题
	if !filepath.IsAbs(dest) {
		var err error
		name, err = filepath.Abs(dest)
		if err != nil {
			return nil, utils.Errorf("get abs path error: %v", err)
		}
	}

	// 简化目录创建逻辑，处理 os.Stat 的所有错误情况
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(name, 0755); err != nil {
				return nil, utils.Errorf("mkdir error: %v", err)
			}
		} else {
			return nil, utils.Errorf("stat error: %v", err) // 返回 os.Stat 的所有错误
		}
	}

	temp := NewRelLocalFs(name)
	if temp == nil { // 检查 NewRelLocalFs 是否返回 nil
		return nil, utils.Errorf("NewRelLocalFs returned nil")
	}

	n, err := Copy(temp, srcFs)
	if err != nil && n <= 0 {
		log.Errorf("copy error: %v", err)               // 记录日志，但仍然返回错误
		return nil, utils.Errorf("copy error: %v", err) // 返回错误给调用者
	}
	return temp, nil
}

func CopyToTemporary(srcFs filesys_interface.FileSystem) *RelLocalFs {
	name := filepath.Join(os.TempDir(), "copied-"+utils.RandStringBytes(12))
	temp := NewRelLocalFs(name)
	_, err := Copy(temp, srcFs)
	if err != nil {
		log.Errorf("copy error: %v", err)
		return temp
	}
	return temp
}

func Copy(destFS, srcFS filesys_interface.FileSystem) (int, error) {
	if utils.IsNil(srcFS) {
		return 0, utils.Errorf("srcFS is nil")
	}
	if utils.IsNil(destFS) {
		return 0, utils.Errorf("destFS is nil")
	}

	var count = new(int64)
	deltaCount := func(i int64) {
		atomic.AddInt64(count, i)
	}
	getCount := func() int64 {
		return atomic.LoadInt64(count)
	}
	err := Recursive(".", WithFileSystem(srcFS), WithStat(func(dir bool, name string, info fs.FileInfo) error {
		if dir {
			err := destFS.MkdirAll(name, 0755)
			if err == nil {
				deltaCount(1)
			}
			return nil
		}
		if ok, err := destFS.Exists(name); err != nil {
			return utils.Errorf("existed check error for: %v: %v", name, err)
		} else if !ok {
			raw, err := srcFS.ReadFile(name)
			if err != nil {
				return utils.Errorf("read error for: %v: %v", name, err)
			}
			err = destFS.WriteFile(name, raw, info.Mode())
			if err != nil {
				log.Warnf("write error for: %v: %v", name, err)
			} else {
				deltaCount(1)
			}
		} else if ok {
			log.Warnf("file already exists: %v auto skip", name)
		}
		return nil
	}))
	if err != nil {
		return int(getCount()), utils.Errorf("copy error: %v", err)
	}
	return int(getCount()), nil
}
