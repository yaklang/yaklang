package ssaapi

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

var (
	ErrContextCancel       error = errors.New("context cancel")
	ErrNoFoundCompiledFile error = errors.New("not found can compiled file")
)

const key = "antlr_cache"

func (c *Config) GetFileHandler(
	filesystem filesys_interface.FileSystem,
	preHandlerFiles []string,
	handlerFilesMap map[string]struct{},
) <-chan *ssareducer.FileContent {
	enableFilePerf := c.GetCompileFilePerformanceLog()

	parse := func(path string, content []byte, store *utils.SafeMap[any]) (ssa.FrontAST, error) {
		var ast ssa.FrontAST
		var err error

		// log.Infof("enableFilePerf %v", enableFilePerf)
		if enableFilePerf {
			profileName := fmt.Sprintf("ParseAST[%s]", normalizePathForProfile(path))
			ssaprofile.ProfileAdd(true, profileName, func() {
				start := time.Now()
				defer func() {
					log.Infof("pre-handler cost:%v parse ast: %s; size(%v)", time.Since(start), path, Size(len(content)))
				}()

				defer func() {
					if r := recover(); r != nil {
						log.Errorf("pre-handler parse [%s] error %v  ", path, r)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()

				if _, needBuild := handlerFilesMap[path]; !needBuild {
					return
				}

				var cache *ssa.AntlrCache
				raw, ok := store.Get(key)
				if ok {
					if raw, ok := raw.(*ssa.AntlrCache); ok && raw != nil {
						cache = raw
					}
				}
				if cache == nil {
					cache = c.LanguageBuilder.GetAntlrCache()
					log.Errorf("get antlr cache from store failed, new one, path: %s, goroutine id: %d", path, getGID())
					store.Set(key, cache)
				}

				if language := c.LanguageBuilder; language != nil {
					if language.FilterParseAST(path) {
						ast, err = language.ParseAST(utils.UnsafeBytesToString(content), cache)
					} else {
						log.Debugf("skip parse ast file: %s", path)
					}
				} else {
					err = utils.Errorf("not select language %s", c.GetLanguage())
				}
			})
		} else {
			// 不启用文件级别监控时，使用原有逻辑
			start := time.Now()
			defer func() {
				log.Infof("pre-handler cost:%v parse ast: %s; size(%v)", time.Since(start), path, Size(len(content)))
			}()

			defer func() {
				if r := recover(); r != nil {
					log.Errorf("pre-handler parse [%s] error %v  ", path, r)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()

			if _, needBuild := handlerFilesMap[path]; !needBuild {
				return nil, nil
			}

			var cache *ssa.AntlrCache
			raw, ok := store.Get(key)
			if ok {
				if raw, ok := raw.(*ssa.AntlrCache); ok && raw != nil {
					cache = raw
				}
			}
			if cache == nil {
				cache = c.LanguageBuilder.GetAntlrCache()
				log.Errorf("get antlr cache from store failed, new one, path: %s, goroutine id: %d", path, getGID())
				store.Set(key, cache)
			}

			if language := c.LanguageBuilder; language != nil {
				if language.FilterParseAST(path) {
					ast, err = language.ParseAST(utils.UnsafeBytesToString(content), cache)
				} else {
					log.Debugf("skip parse ast file: %s", path)
					return nil, nil
				}
			} else {
				err = utils.Errorf("not select language %s", c.GetLanguage())
			}
		}

		return ast, err
	}
	initWorker := func() *utils.SafeMap[any] {
		ret := utils.NewSafeMap[any]()
		ret.Set(key, c.LanguageBuilder.GetAntlrCache())
		log.Debugf("create antrl cache, goroutine id: %d", getGID())
		return ret
	}
	return ssareducer.FilesHandler(
		c.ctx, filesystem, preHandlerFiles,
		parse, initWorker,
		c.astSequence,
		int(c.GetCompileConcurrency()),
	)
}
func getGID() uint64 {
	var buf [64]byte
	// false=不获取全堆栈，仅当前G的ID
	n := runtime.Stack(buf[:], false)
	// 堆栈开头格式: "goroutine 123 [running]:"
	idStr := strings.Fields(strings.TrimPrefix(string(buf[:n]), "goroutine "))[0]
	id, _ := strconv.ParseUint(idStr, 10, 64)
	return id
}

func Size(size int) string {
	if size < 1024 {
		return strconv.Itoa(size) + "B"
	}
	sizeKB := float64(size) / 1024.0
	if sizeKB < 1024 {
		return strconv.FormatFloat(sizeKB, 'f', 2, 64) + "KB"
	}
	sizeMB := sizeKB / 1024.0
	if sizeMB < 1024 {
		return strconv.FormatFloat(sizeMB, 'f', 2, 64) + "MB"
	}
	sizeGB := sizeMB / 1024.0
	return strconv.FormatFloat(sizeGB, 'f', 2, 64) + "GB"
}

// normalizePathForProfile 规范化文件路径用于性能监控名称
// 使用路径的最后两级，避免路径过长
func normalizePathForProfile(path string) string {
	// 处理 Windows 路径分隔符
	path = strings.ReplaceAll(path, "\\", "/")

	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return path
	}
	// 返回最后两级路径
	return strings.Join(parts[len(parts)-2:], "/")
}
