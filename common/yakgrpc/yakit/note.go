package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var _note = &schema.Note{}

func CreateNote(db *gorm.DB, title, content string) (uint, error) {
	note := &schema.Note{
		Title:   title,
		Content: content,
	}
	err := db.Create(note).Error
	return note.ID, err
}

func FilterNote(db *gorm.DB, filter *ypb.NoteFilter) *gorm.DB {
	db = db.Model(_note)
	db = bizhelper.ExactQueryUInt64ArrayOr(db, "id", filter.Id)
	db = bizhelper.ExactQueryStringArrayOr(db, "title", filter.Title)
	keyword := lo.Map(filter.GetKeyword(), func(item string, _ int) any {
		return item
	})
	db = bizhelper.FuzzQueryArrayOrLike(db, "content", keyword, true)
	db = bizhelper.FuzzQueryArrayOrLike(db, "title", keyword, true)
	return db
}

func UpdateNote(db *gorm.DB, filter *ypb.NoteFilter, updateTitle, updateContent bool, title, content string) (int64, error) {
	db = FilterNote(db, filter)
	m := make(map[string]any, 2)
	if updateTitle {
		m["title"] = title
	}
	if updateContent {
		m["content"] = content
	}
	db = db.Updates(m)
	return db.RowsAffected, db.Error
}

func DeleteNote(db *gorm.DB, filter *ypb.NoteFilter) (int64, error) {
	db = FilterNote(db, filter)
	db = db.Unscoped().Delete(_note)
	return db.RowsAffected, db.Error
}

func QueryNote(db *gorm.DB, filter *ypb.NoteFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.Note, error) {
	if paging == nil {
		paging = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	var ret []*schema.Note
	db = bizhelper.QueryOrder(db, paging.OrderBy, paging.Order)
	db = FilterNote(db, filter)
	pag, db := bizhelper.Paging(db, int(paging.Page), int(paging.Limit), &ret)
	return pag, ret, db.Error
}

func SearchNoteContent(db *gorm.DB, keyword string, paging *ypb.Paging) (*bizhelper.Paginator, []*ypb.NoteContent, error) {
	if paging == nil {
		paging = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	var notes []*schema.Note
	db = db.Model(_note)
	db = bizhelper.FuzzQueryArrayOrLike(db, "content", []any{keyword}, true)
	db = bizhelper.QueryOrder(db, paging.OrderBy, paging.Order)
	pag, db := bizhelper.Paging(db, int(paging.Page), int(paging.Limit), &notes)
	ret := lo.FilterMap(notes, func(note *schema.Note, _ int) (*ypb.NoteContent, bool) {
		index := strings.Index(note.Content, keyword)
		if index == -1 {
			log.Errorf("BUG: %s not in %s", keyword, note.Content)
			return nil, false
		}
		lineStart := strings.LastIndexByte(note.Content[:index], '\n')
		if lineStart == -1 {
			lineStart = 0
		}
		lineEnd := strings.IndexByte(note.Content[index:], '\n')
		if lineEnd == -1 {
			lineEnd = len(note.Content)
		} else {
			lineEnd += index
		}
		return &ypb.NoteContent{
			Note:        note.ToGRPCModel(),
			Index:       uint64(index),
			Length:      uint64(len(keyword)),
			LineContent: note.Content[lineStart+1 : lineEnd],
		}, true
	})

	return pag, ret, db.Error
}
