package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Note struct {
	gorm.Model

	Title   string `json:"title"`
	Content string `json:"content"`
}

func (n *Note) ToGRPCModel() *ypb.Note {
	return &ypb.Note{
		Id:       uint64(n.ID),
		Title:    n.Title,
		Content:  n.Content,
		CreateAt: n.CreatedAt.Unix(),
		UpdateAt: n.UpdatedAt.Unix(),
	}
}
