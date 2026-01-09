// Package yaklib 文件监控库
// 支持文件监控、事件捕获、哈希计算、访问日志记录、权限变更检测
// Linux 系统完整支持，Windows/Mac 基础兼容
// 注意：文件类型识别和内容特征匹配功能请使用 file.DetectFileType 等相关函数
package yaklib

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

const (
	FileOpCreate = "create"
	FileOpWrite  = "write"
	FileOpDelete = "delete"
	FileOpChmod  = "chmod"
	FileOpChown  = "chown"
)

// FileAccessLog 文件访问日志结构
type FileAccessLog struct {
	Timestamp   int64  `json:"timestamp"`
	FilePath    string `json:"file_path"`
	Operation   string `json:"operation"`
	User        string `json:"user"`
	UID         int    `json:"uid"`
	GID         int    `json:"gid"`
	FileMode    string `json:"file_mode"`
	FileSize    int64  `json:"file_size"`
	IsDir       bool   `json:"is_dir"`
	Description string `json:"description,omitempty"`
}

// FileMonitorConfig 文件监控配置
type FileMonitorConfig struct {
	WatchPaths      []string                `json:"watch_paths"`
	IncludePatterns []string                `json:"include_patterns"`
	ExcludePatterns []string                `json:"exclude_patterns"`
	Recursive       bool                    `json:"recursive"`
	MonitorOps      []string                `json:"monitor_ops"`
	MaxFileSize     int64                   `json:"max_file_size"`
	LogCallback     func(*FileAccessLog)    `json:"-"`
	EventCallback   func(*FileMonitorEvent) `json:"-"`
}

// FileMonitorEvent 文件监控事件
type FileMonitorEvent struct {
	Type      string    `json:"type"`
	Path      string    `json:"path"`
	IsDir     bool      `json:"is_dir"`
	Timestamp time.Time `json:"timestamp"`
	OldMode   string    `json:"old_mode,omitempty"`
	NewMode   string    `json:"new_mode,omitempty"`
	OldOwner  string    `json:"old_owner,omitempty"`
	NewOwner  string    `json:"new_owner,omitempty"`
	User      string    `json:"user"`
	UID       int       `json:"uid"`
	GID       int       `json:"gid"`
}

type eventRecord struct {
	opType  string
	path    string
	isDir   bool
	info    os.FileInfo
	oldInfo *FileInfo
}

// FileMonitor 文件监控器
type FileMonitor struct {
	config     *FileMonitorConfig
	monitors   map[string]*filesys.YakFileMonitor
	fileHashes *sync.Map // 文件哈希表，用于快速判断文件是否添加/修改/删除
	fileInfos  *sync.Map
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	eventChan  chan *eventRecord
	wg         sync.WaitGroup
}

// FileInfo 文件信息
type FileInfo struct {
	Path    string
	Mode    os.FileMode
	Size    int64
	ModTime time.Time
	IsDir   bool
	UID     int
	GID     int
	Owner   string
	Inode   uint64
	Device  uint64
}

// NewFileMonitor 创建新的文件监控器
func NewFileMonitor(config *FileMonitorConfig) (*FileMonitor, error) {
	if config == nil {
		config = &FileMonitorConfig{
			Recursive:   true,
			MaxFileSize: 10 * 1024 * 1024, // 10MB
		}
	}

	if len(config.WatchPaths) == 0 {
		return nil, utils.Errorf("watch paths cannot be empty")
	}

	ctx, cancel := context.WithCancel(context.Background())

	fm := &FileMonitor{
		config:     config,
		monitors:   make(map[string]*filesys.YakFileMonitor),
		fileHashes: new(sync.Map),
		fileInfos:  new(sync.Map),
		ctx:        ctx,
		cancel:     cancel,
		eventChan:  make(chan *eventRecord, 1000),
	}

	return fm, nil
}

// Start 启动文件监控
func (fm *FileMonitor) Start() error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	fm.wg.Add(1)
	go fm.eventWorker()

	for _, path := range fm.config.WatchPaths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return utils.Errorf("invalid path %s: %v", path, err)
		}

		eventHandler := func(events *filesys.EventSet) {
			fm.handleFileSystemEvents(events)
		}

		monitor, err := filesys.WatchPath(fm.ctx, absPath, eventHandler)
		if err != nil {
			return utils.Errorf("failed to watch path %s: %v", absPath, err)
		}

		fm.monitors[absPath] = monitor
		go fm.initializeFileInfo(absPath)
	}

	return nil
}

// Stop 停止文件监控
func (fm *FileMonitor) Stop() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	fm.performFinalCheck()
	fm.cancel()
	close(fm.eventChan)
	fm.wg.Wait()

	for _, monitor := range fm.monitors {
		if monitor != nil && monitor.CancelFunc != nil {
			monitor.CancelFunc()
		}
	}

	fm.monitors = make(map[string]*filesys.YakFileMonitor)
}

func (fm *FileMonitor) performFinalCheck() {
	fm.fileInfos.Range(func(key, value interface{}) bool {
		path := key.(string)
		info, err := os.Stat(path)
		if err != nil {
			return true
		}

		if !info.IsDir() {
			// 使用哈希表快速判断文件是否被修改
			var oldHash string
			if hash, ok := fm.fileHashes.Load(path); ok {
				oldHash = hash.(string)
			}

			var newHash string
			if info.Size() <= fm.config.MaxFileSize {
				newHash = utils.GetFileMd5(path)
				if newHash != "" {
					fm.fileHashes.Store(path, newHash)
				}
			}

			// 如果哈希值发生变化，说明文件被修改
			if oldHash != newHash {
				newFileInfo := fm.getFileInfo(path, info)
				fm.fileInfos.Store(path, newFileInfo)
				fm.recordEvent(FileOpWrite, path, false, info)
			}
		}

		return true
	})
}

// SetEventCallback 设置事件回调
func (fm *FileMonitor) SetEventCallback(callback func(*FileMonitorEvent)) {
	fm.config.EventCallback = callback
}

// SetLogCallback 设置日志回调
func (fm *FileMonitor) SetLogCallback(callback func(*FileAccessLog)) {
	fm.config.LogCallback = callback
}

func (fm *FileMonitor) GetConfig() *FileMonitorConfig {
	return fm.config
}

func (fm *FileMonitor) initializeFileInfo(path string) {
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			if log.GetLevel() == log.DebugLevel {
				log.Debugf("skip file %s during walk: %v", filePath, err)
			}
			return nil
		}

		if info == nil {
			return nil
		}

		if !fm.shouldMonitor(filePath) {
			return nil
		}

		if info.IsDir() {
			fileInfo := fm.getFileInfo(filePath, info)
			fm.fileInfos.Store(filePath, fileInfo)
			return nil
		}

		if _, statErr := os.Stat(filePath); statErr != nil {
			return nil
		}

		fileInfo := fm.getFileInfo(filePath, info)
		fm.fileInfos.Store(filePath, fileInfo)

		// 计算并存储文件哈希，用于快速判断文件是否被修改
		if info.Size() <= fm.config.MaxFileSize {
			hash := utils.GetFileMd5(filePath)
			if hash != "" {
				fm.fileHashes.Store(filePath, hash)
			}
		}

		return nil
	})

	if err != nil {
		log.Errorf("failed to initialize file info for %s: %v", path, err)
	}
}

func (fm *FileMonitor) shouldMonitor(path string) bool {
	if len(fm.config.IncludePatterns) > 0 {
		matched := false
		for _, pattern := range fm.config.IncludePatterns {
			matched, _ = filepath.Match(pattern, filepath.Base(path))
			if !matched {
				re, err := regexp.Compile(pattern)
				if err == nil && re.MatchString(path) {
					matched = true
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}

	for _, pattern := range fm.config.ExcludePatterns {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if !matched {
			re, err := regexp.Compile(pattern)
			if err == nil && re.MatchString(path) {
				matched = true
			}
		}
		if matched {
			return false
		}
	}

	return true
}

func (fm *FileMonitor) getFileInfo(path string, info os.FileInfo) *FileInfo {
	fileInfo := &FileInfo{
		Path:    path,
		Mode:    info.Mode(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}

	sys := info.Sys()
	if sys != nil {
		v := reflect.ValueOf(sys)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		uidField := v.FieldByName("Uid")
		if uidField.IsValid() && uidField.CanInterface() {
			if uidValue, ok := uidField.Interface().(uint32); ok {
				fileInfo.UID = int(uidValue)
			}

			gidField := v.FieldByName("Gid")
			if gidField.IsValid() && gidField.CanInterface() {
				if gidValue, ok := gidField.Interface().(uint32); ok {
					fileInfo.GID = int(gidValue)
				}
			}

			inoField := v.FieldByName("Ino")
			if inoField.IsValid() && inoField.CanInterface() {
				if inoValue, ok := inoField.Interface().(uint64); ok {
					fileInfo.Inode = inoValue
				}
			}

			devField := v.FieldByName("Dev")
			if devField.IsValid() && devField.CanInterface() {
				if devValue, ok := devField.Interface().(uint64); ok {
					fileInfo.Device = devValue
				}
			}

			if fileInfo.UID > 0 {
				if u, err := user.LookupId(fmt.Sprintf("%d", fileInfo.UID)); err == nil {
					fileInfo.Owner = u.Username
				} else {
					fileInfo.Owner = fmt.Sprintf("uid:%d", fileInfo.UID)
				}
			}
		}
	}

	return fileInfo
}

func (fm *FileMonitor) handleFileSystemEvents(events *filesys.EventSet) {
	if log.GetLevel() == log.DebugLevel {
		log.Debugf("received events: CreateEvents=%d, DeleteEvents=%d, ChangeEvents=%d",
			len(events.CreateEvents), len(events.DeleteEvents), len(events.ChangeEvents))
	}

	for _, event := range events.CreateEvents {
		var info os.FileInfo
		if stat, err := os.Stat(event.Path); err == nil {
			info = stat
			// 计算并存储新创建文件的哈希
			if !event.IsDir && info.Size() <= fm.config.MaxFileSize {
				hash := utils.GetFileMd5(event.Path)
				if hash != "" {
					fm.fileHashes.Store(event.Path, hash)
				}
			}
		}
		fm.recordEvent(FileOpCreate, event.Path, event.IsDir, info)
	}

	for _, event := range events.DeleteEvents {
		var oldFileInfo *FileInfo
		if oldInfo, ok := fm.fileInfos.Load(event.Path); ok {
			oldFileInfo = oldInfo.(*FileInfo)
		}
		fm.recordEventWithOldInfo(FileOpDelete, event.Path, event.IsDir, nil, oldFileInfo)
		if oldFileInfo != nil {
			fm.fileInfos.Delete(event.Path)
			fm.fileHashes.Delete(event.Path) // 清理哈希表
		}
	}

	for _, event := range events.ChangeEvents {
		var info os.FileInfo
		if stat, err := os.Stat(event.Path); err == nil {
			info = stat
			if oldInfo, ok := fm.fileInfos.Load(event.Path); ok {
				oldFileInfo := oldInfo.(*FileInfo)
				newFileInfo := fm.getFileInfo(event.Path, info)

				if oldFileInfo.Mode != newFileInfo.Mode {
					fm.recordEvent(FileOpChmod, event.Path, event.IsDir, info)
				}

				if oldFileInfo.UID != 0 || newFileInfo.UID != 0 {
					if oldFileInfo.UID != newFileInfo.UID || oldFileInfo.GID != newFileInfo.GID {
						fm.recordEvent(FileOpChown, event.Path, event.IsDir, info)
					}
				}

				// 使用哈希表判断文件内容是否被修改
				if !event.IsDir {
					var oldHash string
					if hash, ok := fm.fileHashes.Load(event.Path); ok {
						oldHash = hash.(string)
					}

					var newHash string
					if info.Size() <= fm.config.MaxFileSize {
						newHash = utils.GetFileMd5(event.Path)
						if newHash != "" {
							fm.fileHashes.Store(event.Path, newHash)
						}
					}

					// 如果哈希值发生变化，说明文件内容被修改
					if oldHash != newHash {
						fm.recordEvent(FileOpWrite, event.Path, event.IsDir, info)
					}
				}

				fm.fileInfos.Store(event.Path, newFileInfo)
			} else {
				fm.fileInfos.Store(event.Path, fm.getFileInfo(event.Path, info))
				// 新文件，计算并存储哈希
				if !event.IsDir && info.Size() <= fm.config.MaxFileSize {
					hash := utils.GetFileMd5(event.Path)
					if hash != "" {
						fm.fileHashes.Store(event.Path, hash)
					}
				}
			}
		}
	}

	processedPaths := make(map[string]bool)
	for _, event := range events.CreateEvents {
		processedPaths[event.Path] = true
	}
	for _, event := range events.DeleteEvents {
		processedPaths[event.Path] = true
	}
	for _, event := range events.ChangeEvents {
		processedPaths[event.Path] = true
	}

	fm.fileInfos.Range(func(key, value interface{}) bool {
		path := key.(string)

		if processedPaths[path] {
			return true
		}
		info, err := os.Stat(path)
		if err != nil {
			return true
		}

		if !info.IsDir() {
			// 使用哈希表快速判断文件是否被修改
			var oldHash string
			if hash, ok := fm.fileHashes.Load(path); ok {
				oldHash = hash.(string)
			}

			var newHash string
			if info.Size() <= fm.config.MaxFileSize {
				newHash = utils.GetFileMd5(path)
				if newHash != "" {
					fm.fileHashes.Store(path, newHash)
				}
			}

			// 如果哈希值发生变化，说明文件被修改
			if oldHash != newHash {
				newFileInfo := fm.getFileInfo(path, info)
				fm.fileInfos.Store(path, newFileInfo)
				fm.recordEvent(FileOpWrite, path, false, info)
			}
		}

		return true
	})
}

func (fm *FileMonitor) eventWorker() {
	defer fm.wg.Done()

	for {
		select {
		case <-fm.ctx.Done():
			for {
				select {
				case record, ok := <-fm.eventChan:
					if !ok {
						return
					}
					fm.processEvent(record)
				default:
					return
				}
			}
		case record, ok := <-fm.eventChan:
			if !ok {
				return
			}
			fm.processEvent(record)
		}
	}
}

func (fm *FileMonitor) recordEvent(opType, path string, isDir bool, info os.FileInfo) {
	fm.recordEventWithOldInfo(opType, path, isDir, info, nil)
}

func (fm *FileMonitor) recordEventWithOldInfo(opType, path string, isDir bool, info os.FileInfo, oldInfo *FileInfo) {
	if len(fm.config.MonitorOps) > 0 {
		found := false
		for _, op := range fm.config.MonitorOps {
			if op == opType {
				found = true
				break
			}
		}
		if !found {
			return
		}
	}

	select {
	case fm.eventChan <- &eventRecord{
		opType:  opType,
		path:    path,
		isDir:   isDir,
		info:    info,
		oldInfo: oldInfo,
	}:
	case <-fm.ctx.Done():
	default:
		log.Warnf("event channel full, dropping event: %s %s", opType, path)
	}
}

func (fm *FileMonitor) processEvent(record *eventRecord) {
	event := &FileMonitorEvent{
		Type:      record.opType,
		Path:      record.path,
		IsDir:     record.isDir,
		Timestamp: time.Now(),
	}

	var fileInfo *FileInfo
	var fileMode string

	if record.info != nil {
		// 文件存在，获取当前信息
		fileInfo = fm.getFileInfo(record.path, record.info)
		fileMode = fileInfo.Mode.String()
		event.NewMode = fileMode

		// 优先使用 record.oldInfo（如果提供），否则从缓存中获取
		if record.oldInfo != nil {
			event.OldMode = record.oldInfo.Mode.String()
			event.OldOwner = record.oldInfo.Owner
		} else if oldInfo, ok := fm.fileInfos.Load(record.path); ok {
			oldFileInfo := oldInfo.(*FileInfo)
			event.OldMode = oldFileInfo.Mode.String()
			event.OldOwner = oldFileInfo.Owner
		}

		fm.fileInfos.Store(record.path, fileInfo)
	} else {
		// 文件不存在（如删除事件），优先使用 record.oldInfo
		if record.oldInfo != nil {
			fileInfo = record.oldInfo
			fileMode = fileInfo.Mode.String()
			event.OldMode = fileMode
			event.OldOwner = fileInfo.Owner
		} else if oldInfo, ok := fm.fileInfos.Load(record.path); ok {
			// 如果 record.oldInfo 不存在，尝试从缓存中获取（可能已经被删除）
			oldFileInfo := oldInfo.(*FileInfo)
			fileInfo = oldFileInfo
			fileMode = oldFileInfo.Mode.String()
			event.OldMode = fileMode
			event.OldOwner = oldFileInfo.Owner
		}
	}

	fm.fillUserInfo(event)

	if fm.config.LogCallback != nil {
		accessLog := &FileAccessLog{
			Timestamp: event.Timestamp.Unix(),
			FilePath:  event.Path,
			Operation: event.Type,
			User:      event.User,
			UID:       event.UID,
			GID:       event.GID,
			FileMode:  fileMode,
			IsDir:     event.IsDir,
		}

		if record.info != nil {
			accessLog.FileSize = record.info.Size()
		} else if fileInfo != nil {
			accessLog.FileSize = fileInfo.Size
		}

		if fileInfo != nil {
			if fileInfo.Owner != "" {
				accessLog.User = fileInfo.Owner
			}
			if fileInfo.UID > 0 {
				accessLog.UID = fileInfo.UID
			}
			if fileInfo.GID > 0 {
				accessLog.GID = fileInfo.GID
			}
		}

		fm.config.LogCallback(accessLog)
	}

	if fm.config.EventCallback != nil {
		fm.config.EventCallback(event)
	}
}

func (fm *FileMonitor) fillUserInfo(event *FileMonitorEvent) {
	currentUser, err := user.Current()
	if err == nil {
		event.User = currentUser.Username
		event.UID, _ = strconv.Atoi(currentUser.Uid)
		event.GID, _ = strconv.Atoi(currentUser.Gid)
	}
}

// FileMonitorExports 文件监控导出函数
var FileMonitorExports = map[string]interface{}{
	"NewMonitor": func(config map[string]interface{}) (*FileMonitor, error) {
		monitorConfig := &FileMonitorConfig{
			Recursive:   true,
			MaxFileSize: 10 * 1024 * 1024,
		}

		if paths, ok := config["watch_paths"]; ok {
			paths := utils.InterfaceToStringSlice(paths)
			monitorConfig.WatchPaths = make([]string, 0, len(paths))
			for _, path := range paths {
				if path == "" {
					continue
				}
				monitorConfig.WatchPaths = append(monitorConfig.WatchPaths, path)
			}
		}

		if recursive, ok := config["recursive"]; ok {
			recursive := utils.InterfaceToBoolean(recursive)
			monitorConfig.Recursive = recursive
		}

		if maxSize, ok := config["max_file_size"]; ok {
			maxSize := utils.InterfaceToInt(maxSize)
			monitorConfig.MaxFileSize = int64(maxSize)
		}

		if includes, ok := config["include_patterns"]; ok {
			includes := utils.InterfaceToStringSlice(includes)
			monitorConfig.IncludePatterns = make([]string, 0, len(includes))
			for _, include := range includes {
				if include == "" {
					continue
				}
				monitorConfig.IncludePatterns = append(monitorConfig.IncludePatterns, include)
			}
		}

		if excludes, ok := config["exclude_patterns"]; ok {
			excludes := utils.InterfaceToStringSlice(excludes)
			monitorConfig.ExcludePatterns = make([]string, 0, len(excludes))
			for _, exclude := range excludes {
				if exclude == "" {
					continue
				}
				monitorConfig.ExcludePatterns = append(monitorConfig.ExcludePatterns, exclude)
			}
		}

		if ops, ok := config["monitor_ops"]; ok {
			ops := utils.InterfaceToStringSlice(ops)
			monitorConfig.MonitorOps = make([]string, 0, len(ops))
			for _, op := range ops {
				if op == "" {
					continue
				}
				monitorConfig.MonitorOps = append(monitorConfig.MonitorOps, op)
			}
		}

		return NewFileMonitor(monitorConfig)
	},
}
