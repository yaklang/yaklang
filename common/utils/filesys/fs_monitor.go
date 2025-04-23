package filesys

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

type MonitorEventHandler func(events *EventSet)

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

func (set EventSet) IsEmpty() bool {
	if len(set.CreateEvents) == 0 && len(set.DeleteEvents) == 0 && len(set.ChangeEvents) == 0 {
		return true
	}
	return false
}

type YakFileMonitor struct {
	Events          chan *EventSet
	RecursiveFinish chan struct{} // recursive finish

	FileTreeMutex sync.Mutex
	FileTree      *FileNode

	WatchPatch string

	Ctx        context.Context
	CancelFunc context.CancelFunc
}

func GetCurrentFileTree(path string) (*FileNode, error) {
	initFileNode := func(path string, info os.FileInfo, parent *FileNode, isRoot bool) *FileNode {
		return &FileNode{
			Path:     path,
			Children: make(map[string]*FileNode, 0),
			Parent:   parent,
			IsRoot:   isRoot,
			Info:     info,
		}
	}
	rootInfo, err := os.Stat(path)
	if err != nil {
		return nil, utils.Errorf("failed to watch path: %s", err)
	}
	var currentFileTree = initFileNode(path, rootInfo, nil, true)

	onStat := func(isDir bool, path string, info os.FileInfo) error {
		newNode := initFileNode(path, info, currentFileTree, false)
		currentFileTree.Children[path] = newNode
		if isDir {
			currentFileTree = newNode
			return nil
		} else {
			return SkipAll
		}
	}

	onWalkEnd := func(path string) error {
		if !currentFileTree.IsRoot {
			currentFileTree = currentFileTree.Parent
		}
		return nil
	}

	// init file info map
	err = Recursive(path, WithStat(onStat), WithDirWalkEnd(onWalkEnd))
	if err != nil {
		return nil, utils.Errorf("failed to watch path: %s", err)
	}
	return currentFileTree, nil
}

func (m *YakFileMonitor) UpdateFileTree() error {
	m.FileTreeMutex.Lock()
	defer m.FileTreeMutex.Unlock()
	var err error
	m.FileTree, err = GetCurrentFileTree(m.WatchPatch)
	return err
}

func (m *YakFileMonitor) SetFileTree(fileTree *FileNode) {
	m.FileTreeMutex.Lock()
	defer m.FileTreeMutex.Unlock()
	m.FileTree = fileTree
}

func WatchPath(ctx context.Context, path string, eventHandler MonitorEventHandler) (*YakFileMonitor, error) {
	ctx, cancelFunc := context.WithCancel(ctx)
	m := &YakFileMonitor{
		Events:          make(chan *EventSet, 10),
		Ctx:             ctx,
		CancelFunc:      cancelFunc,
		RecursiveFinish: make(chan struct{}, 10),
		WatchPatch:      path,
	}

	var err error
	m.FileTree, err = GetCurrentFileTree(path)
	if err != nil {
		return nil, utils.Errorf("failed to watch path: %s", err)
	}

	// watch file
	go func() {
		for {
			time.Sleep(1 * time.Second)
			select {
			case <-m.Ctx.Done():
				return
			default:
				currentFileTree, err := GetCurrentFileTree(path)
				if err != nil {
					log.Errorf("failed to watch path: %s", err)
					continue
				}
				m.Events <- CompareFileTree(m.FileTree, currentFileTree)
				m.RecursiveFinish <- struct{}{}
				m.SetFileTree(currentFileTree)
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
				if !events.IsEmpty() {
					eventHandler(events)
				}
			}
		}
	}()

	return m, nil
}

func CompareFileTree(perv, current *FileNode) *EventSet {
	var events = &EventSet{
		CreateEvents: make([]Event, 0),
		DeleteEvents: make([]Event, 0),
		ChangeEvents: make([]Event, 0),
	}
	if perv == nil || current == nil {
		return events
	}
	if !(perv.IsDir() && current.IsDir()) {
		return events
	}

	// 比较当前层级的文件和目录
	for path, currentNode := range current.Children {
		if pervNode, ok := perv.Children[path]; ok {
			// 文件/目录存在于两个树中
			if currentNode.IsDir() != pervNode.IsDir() {
				// 类型改变了（文件变目录或目录变文件）
				events.DeleteEvents = append(events.DeleteEvents, Event{
					Path:  pervNode.Path,
					IsDir: pervNode.IsDir(),
					Op:    FsMonitorDelete,
				})
				events.CreateEvents = append(events.CreateEvents, Event{
					Path:  currentNode.Path,
					IsDir: currentNode.IsDir(),
					Op:    FsMonitorCreate,
				})
			} else if !currentNode.IsDir() {
				// 如果都存在且都是文件，检查文件是否有变化
				if !IsSameFile(currentNode.Info, pervNode.Info) {
					events.ChangeEvents = append(events.ChangeEvents, Event{
						Path:  currentNode.Path,
						IsDir: false,
						Op:    FsMonitorChange,
					})
				}
			} else {
				// 都是目录，递归比较
				subEvents := CompareFileTree(pervNode, currentNode)
				events.CreateEvents = append(events.CreateEvents, subEvents.CreateEvents...)
				events.DeleteEvents = append(events.DeleteEvents, subEvents.DeleteEvents...)
				events.ChangeEvents = append(events.ChangeEvents, subEvents.ChangeEvents...)
			}
		} else {
			// 新增的文件/目录
			events.CreateEvents = append(events.CreateEvents, Event{
				Path:  currentNode.Path,
				IsDir: currentNode.IsDir(),
				Op:    FsMonitorCreate,
			})
		}
	}

	// 检查删除的文件/目录
	for path, pervNode := range perv.Children {
		if _, ok := current.Children[path]; !ok {
			events.DeleteEvents = append(events.DeleteEvents, Event{
				Path:  pervNode.Path,
				IsDir: pervNode.IsDir(),
				Op:    FsMonitorDelete,
			})
		}
	}

	return events
}
