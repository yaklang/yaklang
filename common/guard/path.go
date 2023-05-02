package guard

import (
	"io/ioutil"
	"os"
	"regexp"
	"sync"
	"yaklang/common/log"
	"yaklang/common/utils"
)

type GuardFileInfo struct {
	os.FileInfo

	Path    string
	Content []byte
}

type pathGuardCallback func(old *GuardFileInfo, new *GuardFileInfo)

type PathGuardTarget struct {
	guardTargetBase

	Path         string
	Recursive    bool
	recordOrigin bool

	cacheFileSize         int
	contentChangeCallback pathGuardCallback
	callback              pathGuardCallback
	fileCountLimit        int

	// include/exclude
	includedRegexps []*regexp.Regexp
	excludedRegexps []*regexp.Regexp

	// map[string(Name)]os.FileInfo
	cache           *sync.Map
	isFirst         *utils.AtomicBool
	disallowNewFile *utils.AtomicBool

	origin           *sync.Map
	recordOriginOnce *sync.Once
}

type PathGuardTargetOption func(p *PathGuardTarget) error

func SetPathGuardCacheFileSize(byteSize int) PathGuardTargetOption {
	return func(p *PathGuardTarget) error {
		p.cacheFileSize = byteSize
		return nil
	}
}

func SetPathUnserRecovered(r bool) PathGuardTargetOption {
	return func(p *PathGuardTarget) error {
		p.recordOrigin = r
		return nil
	}
}

func SetPathGuardCallback(f pathGuardCallback) PathGuardTargetOption {
	return func(p *PathGuardTarget) error {
		p.callback = f
		return nil
	}
}

func SetPathGuardContentChangeCallback(f pathGuardCallback) PathGuardTargetOption {
	return func(p *PathGuardTarget) error {
		if p.cacheFileSize <= 0 {
			return utils.Errorf("cache file size is not set")
		}
		p.contentChangeCallback = f
		return nil
	}
}

func SetPathGuardFileCountLimit(i int) PathGuardTargetOption {
	return func(p *PathGuardTarget) error {
		p.fileCountLimit = i
		return nil
	}
}

func SetPathGuardFirstToNotify() PathGuardTargetOption {
	return func(p *PathGuardTarget) error {
		p.isFirst = utils.NewBool(false)
		return nil
	}
}

func SetPathNameIncludes(s ...string) PathGuardTargetOption {
	return func(p *PathGuardTarget) error {
		for _, sub := range s {
			r, err := utils.StarAsWildcardToRegexp("", sub)
			if err != nil {
				return utils.Errorf("compile include rule[%s] failed: %s", sub, err)
			}
			p.includedRegexps = append(p.includedRegexps, r)
		}
		return nil
	}
}

func SetDisallowNewFile(s ...string) PathGuardTargetOption {
	return func(p *PathGuardTarget) error {
		p.disallowNewFile.Set()
		return nil
	}
}

func SetPathNameExcludes(s ...string) PathGuardTargetOption {
	return func(p *PathGuardTarget) error {
		for _, sub := range s {
			r, err := utils.StarAsWildcardToRegexp("", sub)
			if err != nil {
				return utils.Errorf("compile exclude rule[%s] failed: %s", sub, err)
			}
			log.Info(r.String())
			p.excludedRegexps = append(p.excludedRegexps, r)
		}
		return nil
	}
}

func (p *PathGuardTarget) shouldContinueByPath(path string) bool {
	// 如果设置了 include， 则只检查 include 里面的
	if p.includedRegexps != nil {
		shouldContinue := false
		for _, include := range p.includedRegexps {
			if include.MatchString(path) {
				shouldContinue = true
				break
			}
		}
		if !shouldContinue {
			return false
		}
	}

	for _, exclude := range p.excludedRegexps {
		if exclude.MatchString(path) {
			return false
		}
	}
	return true
}

func (p *PathGuardTarget) do() {
	if (p.callback == nil && (p.contentChangeCallback == nil || p.cacheFileSize <= 0)) || p.disallowNewFile.IsSet() {
		return
	}

	state, e := os.Stat(p.Path)
	if e != nil {
		return
	}

	var raw []byte
	if !state.IsDir() && state.Size() <= int64(p.cacheFileSize) {
		raw, _ = ioutil.ReadFile(p.Path)
	}

	var (
		infos = []*GuardFileInfo{{FileInfo: state, Path: p.Path, Content: raw}}
		err   error
	)

	if state.IsDir() {
		var infosRaw []*utils.FileInfo
		if p.Recursive {
			infosRaw, err = utils.ReadFilesRecursivelyWithLimit(p.Path, p.fileCountLimit)
			if err != nil {
				log.Errorf("read %v's recursive failed: %s", p.Path, err)
				return
			}
		} else {
			infosRaw, err = utils.ReadDirWithLimit(p.Path, p.fileCountLimit)
			if err != nil {
				log.Errorf("read dir[%s] failed: %s", p.Path, err)
				return
			}
		}

		for _, r := range infosRaw {
			if !p.shouldContinueByPath(r.Path) {
				continue
			}

			var raw []byte
			if !r.BuildIn.IsDir() && r.BuildIn.Size() <= int64(p.cacheFileSize) {
				raw, _ = ioutil.ReadFile(p.Path)
			}

			infos = append(infos, &GuardFileInfo{
				FileInfo: r.BuildIn,
				Path:     r.Path,
				Content:  raw,
			})
		}
	}

	// 监控文件内容了
	if len(infos) > 0 && p.isFirst.IsSet() {
		defer p.isFirst.UnSet()
	}
	for _, info := range infos {
		//log.Infof("monitor path: %s", info.Path)

		data, ok := p.cache.Load(FileInfoToHash(info))
		if !ok {
			// 如果不允许创建新文件了，就不做缓存，直接删除新建的文件
			if !p.isFirst.IsSet() && p.disallowNewFile.IsSet() {
				log.Infof("disallow to create new file: %v auto deleted", info.Path)
				err := os.RemoveAll(info.Path)
				if err != nil {
					log.Errorf("remove path failed: %s", err)
				}
				continue
			}

			// 如果允许创建新文件
			//    如果有新文件就直接汇报
			p.cache.Store(FileInfoToHash(info), info)
			if (!p.isFirst.IsSet()) && p.callback != nil {
				p.callback(nil, info)
			}

			continue
		}

		oldData := data.(*GuardFileInfo)
		newData := info

		if FileInfoEqual(oldData, newData) {
			continue
		} else {
			p.cache.Store(FileInfoToHash(info), info)
		}

		// 第一次执行，就不要执行 callback 了，不然监控的文件太多会炸掉
		if !p.isFirst.IsSet() {
			if p.callback != nil {
				p.callback(oldData, newData)
			}

			if p.cacheFileSize > 0 && p.contentChangeCallback != nil &&
				utils.CalcSha1(oldData.Content) != utils.CalcSha1(newData.Content) {
				p.contentChangeCallback(oldData, newData)
			}
		}
	}

	// 缓存第一次启动时候的监控文件目录上
	if p.recordOrigin {
		p.recordOriginOnce.Do(func() {
			// 监控文件内容了
			for _, info := range infos {
				if info.IsDir() {
					continue
				}

				if p.cacheFileSize > 0 && info.Size() <= int64(p.cacheFileSize) {
					p.origin.Store(info.Path, info)
				}
			}
		})
	}
}

func FileInfoToHash(c *GuardFileInfo) string {
	return utils.CalcSha1(c.Path, c.IsDir())
}

func FileInfoEqual(old, current *GuardFileInfo) bool {
	return old.Path == current.Path && old.IsDir() == current.IsDir() &&
		old.Mode() == current.Mode() && old.ModTime().Equal(current.ModTime())
}
