package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type Payload struct {
	gorm.Model

	// Must: payload group
	Group string `json:"group" gorm:"index"`

	// payload folder
	Folder     *string `json:"folder" gorm:"column:folder;default:''"`          // default empty string
	GroupIndex *int64  `json:"group_index" gorm:"column:group_index;default:0"` // default 0

	// strconv Quoted
	// Must: payload data
	Content *string `json:"content"`

	// hit count
	HitCount *int64 `json:"hit_count" gorm:"column:hit_count;default:0"` // default 0

	// the group save in file only contain one payload, and this `payload.IsFile = true` `payload.Content` is filepath
	IsFile *bool `json:"is_file" gorm:"column:is_file;default:false"` // default false

	// Hash string
	Hash string `json:"hash" gorm:"unique_index"`
}

func (p *Payload) CalcHash() string {
	content := ""
	folder := ""
	if p.Content != nil {
		content = *p.Content
	}
	if p.Folder != nil {
		folder = *p.Folder
	}
	return utils.CalcSha1(p.Group, content, folder)
}

func (p *Payload) BeforeUpdate() error {
	p.Hash = p.CalcHash()
	return nil
}

func (p *Payload) BeforeSave() error {
	p.Hash = p.CalcHash()
	return nil
}

func (p *Payload) BeforeCreate() error {
	p.Hash = p.CalcHash()
	return nil
}
