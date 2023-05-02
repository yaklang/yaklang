package yakgrpc

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yakgrpc/yakit"
)

type MITMFilterManager struct {
	db               *gorm.DB `json:"-"`
	IncludeHostnames []string `json:"includeHostnames"`
	ExcludeHostnames []string `json:"excludeHostnames"`
	IncludeSuffix    []string `json:"includeSuffix"`
	ExcludeSuffix    []string `json:"excludeSuffix"`
	ExcludeMethods   []string `json:"excludeMethods"`
	ExcludeMIME      []string `json:"excludeMIME"`
	ExcludeUri       []string `json:"excludeUri"`
	IncludeUri       []string `json:"includeUri"`
}

var defaultExcludeHostnames = []string{"google.com", "*gstatic.com", "*bdstatic.com", "*google*.com"}
var defaultExcludeSuffix = []string{
	".css",
	".jpg", ".jpeg", ".png",
	".mp3", ".mp4", ".ico", ".bmp",
	".flv", ".aac", ".ogg", "avi",
	".svg", ".gif", ".woff", ".woff2",
	".doc", ".docx", ".pptx",
	".ppt", ".pdf"}
var defaultExcludeMethods = []string{"OPTIONS", "CONNECT"}
var defaultExcludeMIME = []string{
	"image/*",
	"audio/*", "video/*", "*octet-stream*",
	"application/ogg", "application/pdf", "application/msword",
	"application/x-ppt", "video/avi", "application/x-ico",
	"*zip",
}

func (m *MITMFilterManager) Recover() {
	m.ExcludeMethods = defaultExcludeMethods
	m.ExcludeSuffix = defaultExcludeSuffix
	m.ExcludeHostnames = defaultExcludeHostnames
	m.ExcludeMIME = defaultExcludeMIME
	m.ExcludeUri = nil
	m.IncludeUri = nil
	m.IncludeHostnames = nil
	m.IncludeSuffix = nil
	m.Save()
}

var defaultMITMFilterManager = &MITMFilterManager{
	ExcludeHostnames: defaultExcludeHostnames,
	ExcludeSuffix:    defaultExcludeSuffix,
	ExcludeMethods:   defaultExcludeMethods,
	ExcludeMIME:      defaultExcludeMIME,
}

func getInitFilterManager(db *gorm.DB) func() (*MITMFilterManager, error) {
	if db == nil {
		return nil
	}
	return func() (*MITMFilterManager, error) {
		if db == nil {
			return nil, utils.Error("no database set")
		}
		results := yakit.GetKey(db, MITMFilterKeyRecords)
		var manager MITMFilterManager
		err := json.Unmarshal([]byte(results), &manager)
		if err != nil {
			return nil, err
		}
		managerP := &manager
		//managerP.saveHandler = func(filter *MITMFilterManager) {
		//
		//}
		return managerP, nil
	}
}

func NewMITMFilterManager(db *gorm.DB) *MITMFilterManager {
	var initFilter = getInitFilterManager(db)
	if initFilter == nil {
		defaultMITMFilterManager.db = db
		return defaultMITMFilterManager
	}
	result, err := initFilter()
	if err != nil || result == nil {
		defaultMITMFilterManager.db = db
		return defaultMITMFilterManager
	}
	result.db = db
	return result
}

func (m *MITMFilterManager) IsEmpty() bool {
	return len(m.ExcludeMIME) <= 0 && len(m.ExcludeMethods) <= 0 &&
		len(m.ExcludeSuffix) <= 0 && len(m.ExcludeHostnames) <= 0 &&
		len(m.IncludeHostnames) <= 0 && len(m.IncludeSuffix) <= 0
}

func (m *MITMFilterManager) Save() {
	db := m.db
	if db == nil {
		return
	}

	if m.IsEmpty() {
		m.Recover()
		return
	}

	result, err := json.Marshal(m)
	if err != nil {
		log.Errorf("marshal mitm filter failed: %s", err)
		return
	}
	err = yakit.SetKey(db, MITMFilterKeyRecords, string(result))
	if err != nil {
		log.Errorf("set filter db key failed: %s", err)
	}
}
