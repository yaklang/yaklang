package yakit

import (
	"bufio"
	"context"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
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

func (p *Payload) BeforeUpdate() (err error) {
	p.Hash = p.CalcHash()
	return
}

func (p *Payload) BeforeSave() error {
	p.Hash = p.CalcHash()
	return nil
}

type gormNoLog int

func (i gormNoLog) Print(v ...interface{}) {
}

// / payload
func NewPayload(group string, content string) *Payload {
	s := ""
	var h int64 = 0
	var f bool = false
	p := &Payload{
		Group:    group,
		Content:  &content,
		Folder:   &s,
		HitCount: &h,
		IsFile:   &f,
	}
	p.Hash = p.CalcHash()
	return p
}

func QueryPayloadWithCallBack(db *gorm.DB, p *Payload, notExistCallback, existCallback func(*gorm.DB, *Payload) error) error {
	db = db.Model(&Payload{})
	p.Hash = p.CalcHash()
	var (
		count int64
		err   error
	)
	if db.Where("`hash` = ?", p.Hash).Count(&count); count > 0 {
		err = existCallback(db, p)
	} else {
		err = notExistCallback(db, p)
	}

	return err
}

func createOrUpdatePayload(db *gorm.DB, p *Payload) error {
	return QueryPayloadWithCallBack(
		db,
		p,
		func(db *gorm.DB, i *Payload) error {
			return db.Create(&i).Error
		},
		func(db *gorm.DB, i *Payload) error {
			return db.Where("`hash` = ?", i.Hash).Updates(map[string]any{"hit_count": i.HitCount, "group_index": i.GroupIndex}).Error
		})
}

func updateOrDeletePayload(db *gorm.DB, p *Payload) error {
	return QueryPayloadWithCallBack(
		db,
		p,
		func(db *gorm.DB, p *Payload) error {
			return UpdatePayload(db, int(p.ID), p)
		},
		func(db *gorm.DB, p *Payload) error {
			return DeletePayloadByID(db, int64(p.ID))
		})
}

func CreateOrUpdatePayload(db *gorm.DB, content, group, folder string, hitCount int64, isFile bool) error {
	payload := NewPayload(group, content)
	payload.Folder = &folder
	payload.HitCount = &hitCount
	payload.IsFile = &isFile
	return createOrUpdatePayload(db, payload)
}

// trim payload content
func TrimWhitespaceExceptSpace(r rune) bool {
	if uint32(r) <= '\u00FF' {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', 0x85, 0xA0:
			return true
		}
		return false
	}
	return false
}

func CheckExistGroup(db *gorm.DB, group, folder string) (bool, error) {
	var count int64
	if db := db.Model(&Payload{}).Where("`group` = ?", group).Where("`folder` = ?", folder).Count(&count); db.Error != nil {
		return false, db.Error
	}
	return count > 0, nil
}

// save payload from file
func SavePayloadByFilename(db *gorm.DB, group string, fileName string) error {
	return SavePayloadByFilenameEx(fileName, func(s string, hitCount int64) error {
		return CreateOrUpdatePayload(db, s, group, "", hitCount, true)
	})
}

func SavePayloadByFilenameEx(fileName string, handler func(string, int64) error) error {
	ch, err := utils.FileLineReader(fileName)
	if err != nil {
		return err
	}

	firstLine := true
	isCSV := strings.HasSuffix(fileName, ".csv")
	for lineRaw := range ch {
		line := utils.UnsafeBytesToString(lineRaw)
		var hitCount int64 = 0
		if isCSV {
			if firstLine {
				firstLine = false
			} else {
				lines := utils.PrettifyListFromStringSplited(line, ",")
				if len(lines) == 0 {
					continue
				}
				p := strconv.Quote(strings.TrimRightFunc(lines[0], TrimWhitespaceExceptSpace))
				if len(lines) > 1 {
					// hit count
					i, err := strconv.ParseInt(lines[1], 10, 64)
					if err == nil {
						hitCount = i
					}
				}
				if err := handler(p, hitCount); err != nil {
					return err
				}
			}
		} else {
			line = strconv.Quote(strings.TrimRightFunc(line, TrimWhitespaceExceptSpace))
			if err := handler(line, hitCount); err != nil {
				return err
			}
		}
	}
	return nil
}

// save payload from data
func SavePayloadGroup(db *gorm.DB, group string, lists []string) error {
	for _, i := range lists {
		p := NewPayload(group, strconv.Quote(i))
		err := createOrUpdatePayload(db, p)
		if err != nil {
			return err
		}
	}
	return nil
}

// save payload from raw-data
func SavePayloadGroupByRaw(db *gorm.DB, group string, data string) error {
	return HandleData(data, func(s string) error {
		return CreateOrUpdatePayload(db, s, group, "", 0, false)
	})
}

func HandleData(data string, handler func(string) error) error {
	r := bufio.NewReader(strings.NewReader(data))
	for {
		lineRaw, err := utils.BufioReadLine(r)
		if err != nil {
			break
		}
		line := strconv.Quote(strings.TrimRightFunc(utils.UnsafeBytesToString(lineRaw), TrimWhitespaceExceptSpace))
		if err := handler(line); err != nil {
			log.Errorf("create or update payload error: %s", err.Error())
			continue
		}
	}
	return nil
}

func GetPayloadById(db *gorm.DB, id int64) (*Payload, error) {
	var req Payload
	if db := db.Model(&Payload{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Payload failed: %s", db.Error)
	}

	return &req, nil
}

func GetPayloadsByGroup(db *gorm.DB, group string) ([]*Payload, error) {
	var reqs []*Payload
	if db := db.Model(&Payload{}).Where("`group` = ?", group).Find(&reqs); db.Error != nil {
		return nil, utils.Errorf("get Payload failed: %s", db.Error)
	}
	return reqs, nil
}

func GetPayloadsByFolder(db *gorm.DB, folder string) ([]*Payload, error) {
	var reqs []*Payload
	if db := db.Model(&Payload{}).Where("`folder` = ?", folder).Find(&reqs); db.Error != nil {
		return nil, utils.Errorf("get Payload by folder failed: %s", db.Error)
	}
	return reqs, nil
}

func SetGroupInEnd(db *gorm.DB, group string) error {
	var groups []string
	if err := db.Model(&Payload{}).Select("`group`").Group("`group`").Pluck("`group`", &groups).Error; err != nil {
		return err
	}
	// 更新group_index
	if err := db.Model(&Payload{}).Where("`group` = ?", group).Update("group_index", len(groups)+1).Error; err != nil {
		return err
	}
	return nil
}

func GetPayloadByGroupFirst(db *gorm.DB, group string) (*Payload, error) {
	var req Payload
	if db := db.Model(&Payload{}).Where("`group` = ?", group).First(&req); db.Error != nil {
		return nil, utils.Wrapf(db.Error, "get Payload by group %s failed", group)
	} else {
		return &req, nil
	}
}

func GetPayloadGroupFileName(db *gorm.DB, group string) (string, error) {
	if payload, err := GetPayloadByGroupFirst(db, group); err != nil {
		return "", err
	} else {
		if payload.IsFile != nil && *payload.IsFile {
			return *payload.Content, nil
		} else {
			return "", utils.Errorf("this group %s save in database not in file", group)
		}
	}
}

func GetPayloadCountInGroup(db *gorm.DB, group string) int64 {
	var i int64
	if db := db.Model(&Payload{}).Where("`group` = ?", group).Count(&i); db.Error != nil {
		return 0
	}
	return i
}

func DeletePayloadByID(db *gorm.DB, id int64) error {
	if db := db.Where(
		"id = ?", id,
	).Unscoped().Delete(&Payload{}); db.Error != nil {
		return db.Error
	} else {
		return nil
	}
}

func DeletePayloadByIDs(db *gorm.DB, id []int64) error {
	if db := bizhelper.ExactQueryInt64ArrayOr(db, "id", id).Unscoped().Delete(&Payload{}); db.Error != nil {
		return db.Error
	} else {
		return nil
	}
}

func DeletePayloadByGroup(db *gorm.DB, group string) error {
	if db := db.Model(&Payload{}).Where(
		"`group` = ?", group,
	).Unscoped().Delete(&Payload{}); db.Error != nil {
		return db.Error
	} else {
		return nil
	}
}

func DeletePayloadByFolder(db *gorm.DB, folder string) error {
	if db := db.Model(&Payload{}).Where(
		"`folder` = ?", folder,
	).Unscoped().Delete(&Payload{}); db.Error != nil {
		return db.Error
	} else {
		return nil
	}
}

func RenamePayloadFolder(db *gorm.DB, folder, newFolder string) error {
	db = db.Model(&Payload{}).Where("`folder` = ?", folder)
	gen := YieldPayloads(db, context.Background())
	for payload := range gen {
		payload.Folder = &newFolder
		if err := UpdatePayload(db, int(payload.ID), payload); err != nil {
			return utils.Wrap(err, "update payload error")
		}
	}
	return nil
}

func RenamePayloadGroup(db *gorm.DB, oldGroup, newGroup string) error {
	for payload := range YieldPayloads(db.Model(&Payload{}).Where("`group` = ?", oldGroup), context.Background()) {
		payload.Group = newGroup
		if err := UpdatePayload(db, int(payload.ID), payload); err != nil {
			return utils.Wrap(err, "update payload error")
		}
	}
	return nil
}

func CopyPayloads(db *gorm.DB, payloads []*Payload, group, folder string) error {
	for _, payload := range payloads {
		payload.ID = 0
		payload.Group = group
		payload.Folder = &folder
		if err := createOrUpdatePayload(db, payload); err != nil {
			return utils.Wrap(err, "creating new payload error")
		}
	}
	return nil
}

func MovePayloads(db *gorm.DB, payloads []*Payload, group, folder string) error {
	for _, payload := range payloads {
		payload.Group = group
		payload.Folder = &folder
		if err := updateOrDeletePayload(db, payload); err != nil {
			return utils.Wrap(err, "update payload error")
		}
	}
	return nil
}

func SetIndexToFolder(db *gorm.DB, folder, group string, group_index int64) error {
	db = db.Model(&Payload{})
	// 查找或创建一个新的记录
	payload := Payload{
		Folder:     &folder,
		Group:      group,
		GroupIndex: &group_index,
	}
	db = db.FirstOrCreate(&payload, Payload{Folder: &folder, Group: group})

	// 如果创建失败，返回错误
	if db.Error != nil {
		return utils.Wrap(db.Error, "create or find payload failed")
	}

	// 更新group_index
	gen := YieldPayloads(db.Where("`folder` = ?", folder).Where("`group` = ?", group), context.Background())
	for p := range gen {
		p.GroupIndex = &group_index
		if err := createOrUpdatePayload(db, p); err != nil {
			return utils.Wrap(err, "error creating new payload")
		}
	}
	return nil
}

func UpdatePayloadGroup(db *gorm.DB, group, folder string, group_index int64) error {
	gen := YieldPayloads(db.Where("`group` = ?", group), context.Background())
	for p := range gen {
		p.GroupIndex = &group_index
		if err := UpdatePayload(db, int(p.ID), p); err != nil {
			return utils.Wrap(err, "creating new payload error")
		}
	}
	return nil
}

func UpdatePayload(db *gorm.DB, id int, payload *Payload) error {
	payload.ID = uint(id)
	payload.Hash = payload.CalcHash()
	db = db.Model(&Payload{}).Where("`id` = ?", id)
	db = db.Updates(map[string]any{"group": payload.Group, "folder": payload.Folder, "group_index": payload.GroupIndex, "content": payload.Content, "hit_count": payload.HitCount, "is_file": payload.IsFile, "hash": payload.Hash})
	if db.Error != nil {
		return utils.Errorf("update Payload failed: %s", db.Error)
	}
	return nil
}

func PayloadGroups(db *gorm.DB, search ...string) []string {
	if len(search) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "`group`", search)
	}
	rows, err := db.Model(&Payload{}).Select("distinct `group`").Rows()
	if err != nil {
		log.Errorf("query distinct payload group failed: %s", err)
		return []string{}
	}
	var groups []string
	for rows.Next() {
		var group string
		err := rows.Scan(&group)
		if err != nil {
			log.Errorf("scan group failed: %s", err)
			return groups
		}
		groups = append(groups, group)
	}
	return groups
}

type Paging struct {
	OrderBy string
	Order   string
	Page    int
	Limit   int
}

func NewPaging() *Paging {
	return &Paging{
		OrderBy: "id",
		Order:   "asc",
		Page:    1,
		Limit:   30,
	}
}

func QueryPayloadWithoutPaging(db *gorm.DB, folder, group, keyword string) ([]*Payload, error) {
	db = db.Model(&Payload{})
	if group != "" {
		db = db.Where("`group` = ?", group)
	}
	if folder != "" {
		db = db.Where("`folder` = ?", folder)
	}
	if keyword != "" {
		db = db.Where("`content` = ?", keyword)
	}

	var ret []*Payload
	db = db.Find(&ret)
	if db.Error != nil {
		return nil, utils.Errorf("query payload failed: %s", db.Error)
	}
	return ret, nil
}

func QueryPayload(db *gorm.DB, folder, group, keyword string, paging *Paging) (*bizhelper.Paginator, []*Payload, error) {
	db = db.Model(&Payload{}) // .Debug()
	db = bizhelper.QueryOrder(db, paging.OrderBy, paging.Order)
	db = bizhelper.ExactQueryString(db, "`folder`", folder)
	db = bizhelper.ExactQueryString(db, "`group`", group)
	// db = bizhelper.QueryByBool(db, "`is_file`", false)
	db = bizhelper.FuzzQueryLike(db, "content", keyword)
	var ret []*Payload
	pag, db := bizhelper.Paging(db, paging.Page, paging.Limit, &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, ret, nil
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
