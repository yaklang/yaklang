package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type ScreenRecorder struct {
	gorm.Model

	// 保存到本地的路径
	Filename  string
	NoteInfo  string
	Project   string
	Hash      string `json:"hash" gorm:"unique_index"`
	VideoName string
	Cover     string `gorm:"type:longtext"`
	Duration  string
}

func (s *ScreenRecorder) CalcHash() string {
	s.Hash = utils.CalcSha1(s.Filename, s.Project)
	return s.Hash
}

func (s *ScreenRecorder) BeforeSave() error {
	s.Hash = s.CalcHash()
	return nil
}
