package payloads

import (
	"github.com/yaklang/yaklang/common/log"
	"strconv"
	"strings"
	"time"
)

type FileBaseInfo struct {
	Filename   string `json:"filename"`
	Time       string `json:"time"`
	Size       string `json:"size"`
	Type       string `json:"type"`
	Permission string `json:"current_user_permissions"`
}

func (f *FileBaseInfo) GetSize() int {
	size, err := strconv.Atoi(f.Size)
	if err != nil {
		return 0
	} else {
		return size
	}
}

func (f *FileBaseInfo) GetTime() int64 {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		log.Errorf("cannot load location Asia/Shanghai: %s", err)
		return time.Now().Unix()
	}
	location, err := time.ParseInLocation("2006-01-02 15:04:05", f.Time, loc)
	if err != nil {
		log.Errorf("time parse fail: %s", err)
		return time.Now().Unix()
	}
	return location.Unix()
}

func (f *FileBaseInfo) HasChildNodes() bool {
	if strings.Contains(strings.ToLower(f.Type), "dir") {
		return true
	}
	return false
}
