package schema

import (
	"context"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

func (p *Payload) GetContent() string {
	if p == nil || p.Content == nil {
		return ""
	}
	content := *p.Content
	unquoted, err := strconv.Unquote(content)
	if err == nil {
		content = unquoted
	}
	content = strings.TrimSpace(content)
	return content
}

func (p *Payload) GetIsFile() bool {
	if p == nil || p.IsFile == nil {
		return false
	}
	return *p.IsFile
}

func (p *Payload) ToGRPCModel() *ypb.Payload {
	content := p.GetContent()
	model := &ypb.Payload{
		Id:           int64(p.ID),
		Group:        p.Group,
		ContentBytes: []byte(content),
		Content:      utils.EscapeInvalidUTF8Byte([]byte(content)),
	}
	if p.Folder != nil {
		model.Folder = *p.Folder
	}
	if p.HitCount != nil {
		model.HitCount = *p.HitCount
	}
	if p.IsFile != nil {
		model.IsFile = *p.IsFile
	}
	return model
}

func NewPayloadFromGRPCModel(p *ypb.Payload) *Payload {
	content := strconv.Quote(p.Content)
	payload := &Payload{
		Group:    p.Group,
		Content:  &content,
		Folder:   &p.Folder,
		HitCount: &p.HitCount,
		IsFile:   &p.IsFile,
	}
	payload.Hash = payload.CalcHash()
	return payload
}

func YieldPayloads(db *gorm.DB, ctx context.Context) chan *Payload {
	outC := make(chan *Payload)
	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*Payload
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}
