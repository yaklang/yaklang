package filesys

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	FsMonitorCreate = "create" // dir
	FsMonitorWrite  = "write"
	FsMonitorRename = "rename"
	FsMonitorRemove = "remove"
	FsMonitorChmod  = "chmod"
	FsMonitorChange = "change"
	FsMonitorTouch  = "touch"
	FsMonitorDelete = "delete"
)

type MonitorEventHandler func(events EventSet)

// type MonitorErrorsHandler func(error)
type Event struct {
	Path  string
	Op    string
	IsDir bool
}

type EventSet struct {
	CreateEvents []Event
	DeleteEvents []Event
	ChangeEvents []Event
}

type YakFileMonitor struct {
	Events          chan EventSet
	RecursiveFinish chan struct{} // recursive finish

	FileInfoMutex sync.Mutex
	FileInfos     map[string]os.FileInfo

	Ctx        context.Context
	CancelFunc context.CancelFunc
}

func WatchPath(ctx context.Context, path string, eventHandler MonitorEventHandler) (*YakFileMonitor, error) {
	ctx, cancelFunc := context.WithCancel(ctx)
	m := &YakFileMonitor{
		Events:          make(chan EventSet, 10),
		FileInfos:       make(map[string]os.FileInfo),
		Ctx:             ctx,
		CancelFunc:      cancelFunc,
		RecursiveFinish: make(chan struct{}, 10),
	}

	var currentFileInfo = make(map[string]os.FileInfo, 0)

	watchStat := func(isDir bool, path string, info os.FileInfo) error {
		currentFileInfo[path] = info
		return nil
	}

	startOnStat := func(isDir bool, path string, info os.FileInfo) error {
		m.FileInfoMutex.Lock()
		defer m.FileInfoMutex.Unlock()
		m.FileInfos[path] = info
		return nil
	}

	// init file info map
	err := Recursive(path, WithStat(startOnStat))
	if err != nil {
		return nil, utils.Errorf("failed to watch path: %s", err)
	}

	// watch file
	go func() {
		for {
			select {
			case <-m.Ctx.Done():
				return
			default:
				err = Recursive(path, WithStat(watchStat))
				m.Events <- CalculateFileChange(m.FileInfos, currentFileInfo)
				m.RecursiveFinish <- struct{}{}
				m.FileInfoMutex.Lock()
				m.FileInfos = currentFileInfo
				m.FileInfoMutex.Unlock()
				currentFileInfo = make(map[string]os.FileInfo)
				if err != nil {
					log.Errorf("failed to watch path: %s", err)
				}
				time.Sleep(2 * time.Second)
			}
		}
	}()

	//process event
	go func() {
		for {
			select {
			case <-m.Ctx.Done():
				return
			case <-m.RecursiveFinish:
				events := <-m.Events
				eventHandler(events)
			}
		}
	}()

	return m, nil
}

func CalculateFileChange(perv, current map[string]fs.FileInfo) EventSet {
	var eventSet EventSet
	prevCopy := utils.CopyMapShallow(perv)
	currentCopy := utils.CopyMapShallow(current)
	for k, v := range currentCopy {
		if pervFileInfo, ok := prevCopy[k]; ok {
			if !pervFileInfo.IsDir() && pervFileInfo.ModTime() != v.ModTime() {
				eventSet.ChangeEvents = append(eventSet.ChangeEvents, Event{Path: k, Op: FsMonitorChange}) // file change
			}
			delete(prevCopy, k)
			delete(currentCopy, k)
		}
	}
	for k, _ := range currentCopy {
		eventSet.CreateEvents = append(eventSet.CreateEvents, Event{Path: k, Op: FsMonitorCreate})
	}
	for k, _ := range prevCopy {
		eventSet.DeleteEvents = append(eventSet.DeleteEvents, Event{Path: k, Op: FsMonitorDelete})
	}
	return eventSet
}

// 提供给 yakurl 的不触发事件的修改接口
func (m *YakFileMonitor) Delete(path string) {
	m.FileInfoMutex.Lock()
	defer m.FileInfoMutex.Unlock()
	delete(m.FileInfos, path)
}

func (m *YakFileMonitor) Create(path string, info fs.FileInfo) {
	m.FileInfoMutex.Lock()
	defer m.FileInfoMutex.Unlock()
	m.FileInfos[path] = info
}

func (m *YakFileMonitor) Rename(path string, newname string, info fs.FileInfo) {
	m.FileInfoMutex.Lock()
	defer m.FileInfoMutex.Unlock()
	if !info.IsDir() {
		delete(m.FileInfos, path)
		m.FileInfos[newname] = info
	}
	for k, _ := range m.FileInfos {
		if strings.HasPrefix(k, path) {
			delete(m.FileInfos, k)
			newPath := strings.Replace(k, path, newname, 1)
			newInfo, err := os.Stat(newPath)
			if err == nil {
				m.FileInfos[newPath] = newInfo
			}
		}
	}
}
