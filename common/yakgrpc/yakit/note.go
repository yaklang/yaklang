package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var _note = &schema.Note{}

func CreateNote(db *gorm.DB, title, content string) (uint, error) {
	note := &schema.Note{
		Title:   utils.EscapeInvalidUTF8Byte([]byte(title)),
		Content: utils.EscapeInvalidUTF8Byte([]byte(content)),
	}
	err := db.Create(note).Error
	return note.ID, err
}

func FilterNote(db *gorm.DB, filter *ypb.NoteFilter) *gorm.DB {
	db = db.Model(_note)
	db = bizhelper.ExactQueryUInt64ArrayOr(db, "id", filter.Id)
	db = bizhelper.FuzzQueryStringArrayOr(db, "title", filter.Title)
	keyword := lo.Map(filter.GetKeyword(), func(item string, _ int) any {
		return item
	})
	db = bizhelper.FuzzQueryOrEx(db, []string{"content", "title"}, keyword, true)
	return db
}

func UpdateNote(db *gorm.DB, filter *ypb.NoteFilter, updateTitle, updateContent bool, title, content string) (int64, error) {
	db = FilterNote(db, filter)
	m := make(map[string]any, 2)
	if updateTitle {
		m["title"] = utils.EscapeInvalidUTF8Byte([]byte(title))
	}
	if updateContent {
		m["content"] = utils.EscapeInvalidUTF8Byte([]byte(content))
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

	ret := make([]*ypb.NoteContent, 0, len(notes))

	if keyword == "" {
		return pag, ret, nil
	}

	for _, note := range notes {
		content := note.Content
		editor := memedit.NewMemEditor(content)
		lineMap := make(map[int]struct{})

		editor.FindStringRange(keyword, func(ri *memedit.Range) error {
			start := ri.GetStart()
			startLine := start.GetLine()
			line, err := editor.GetLine(startLine)
			if err != nil {
				line = ri.String()
			}
			if _, ok := lineMap[startLine]; !ok {
				lineMap[startLine] = struct{}{}
			} else {
				return nil
			}
			ret = append(ret, &ypb.NoteContent{
				Note:        note.ToGRPCModel(),
				Line:        uint64(startLine),
				Index:       uint64(editor.GetOffsetByPosition(start)),
				LineContent: line,
			})

			return nil
		})
	}

	return pag, ret, db.Error
}
