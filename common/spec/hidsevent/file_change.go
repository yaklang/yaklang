package hidsevent

import (
	"github.com/fsnotify/fsnotify"
	"os"
	"time"
)

type FileChangeInfo struct {
	IsDir             bool        `json:"is_dir"`
	Path              string      `json:"path"`
	Name              string      `json:"name"`
	CurrentFileMode   os.FileMode `json:"current_file_mode"`
	OriginFileMode    os.FileMode `json:"origin_file_mode"`
	Op                string      `json:"op"`
	OriginData        []byte      `json:"origin_data"`
	CurrentData       []byte      `json:"current_data"`
	OriginModifyTime  time.Time   `json:"origin_modify_time"`
	CurrentModifyTime time.Time   `json:"current_modify_time"`
}

const (
	FsNotifyCreate = "create"
	FsNotifyWrite  = "write"
	FsNotifyRename = "rename"
	FsNotifyRemove = "remove"
	FsNotifyChmod  = "chmod"
	FsNotifyChange = "change"
	FsNotifyTouch  = "touch"
)

func FsNotifyOpToString(i fsnotify.Op) string {
	switch i {
	case fsnotify.Create:
		return "create"
	case fsnotify.Write:
		return "write"
	case fsnotify.Rename:
		return "rename"
	case fsnotify.Remove:
		return "remove"
	case fsnotify.Chmod:
		return "chmod"
	default:
		return ""
	}
}
