package yakit

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

// / payload
func NewPayload(group string, content string) *schema.Payload {
	s := ""
	var h int64 = 0
	var f bool = false
	p := &schema.Payload{
		Group:    group,
		Content:  &content,
		Folder:   &s,
		HitCount: &h,
		IsFile:   &f,
	}
	p.Hash = p.CalcHash()
	return p
}

func QueryPayloadWithCallBack(db *gorm.DB, p *schema.Payload, notExistCallback, existCallback func(*gorm.DB, *schema.Payload) error) error {
	db = db.Model(&schema.Payload{})
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

func createOrUpdatePayload(db *gorm.DB, p *schema.Payload) error {
	return QueryPayloadWithCallBack(
		db,
		p,
		func(db *gorm.DB, i *schema.Payload) error {
			i.Hash = i.CalcHash()
			return db.Create(&i).Error
		},
		func(db *gorm.DB, i *schema.Payload) error {
			return db.Where("`hash` = ?", i.Hash).Updates(map[string]any{"hit_count": i.HitCount, "group_index": i.GroupIndex}).Error
		})
}

func updateOrDeletePayload(db *gorm.DB, p *schema.Payload) error {
	return QueryPayloadWithCallBack(
		db,
		p,
		func(db *gorm.DB, p *schema.Payload) error {
			return UpdatePayload(db, int(p.ID), p)
		},
		func(db *gorm.DB, p *schema.Payload) error {
			return DeletePayloadByID(db, int64(p.ID))
		})
}

func CreatePayload(db *gorm.DB, content, group, folder string, hitCount int64, isFile bool) error {
	payload := NewPayload(group, content)
	payload.Folder = &folder
	payload.HitCount = &hitCount
	payload.IsFile = &isFile
	payload.Hash = payload.CalcHash()
	return db.Create(&payload).Error
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

func CheckExistGroup(db *gorm.DB, group string) (*schema.Payload, error) {
	var (
		payload schema.Payload
	)
	if db := db.Model(&schema.Payload{}).Select("folder, is_file").Where("`group` = ?", group).First(&payload); db.Error != nil {
		return nil, db.Error
	}
	return &payload, nil
}

// save payload from file
func SavePayloadByFilename(db *gorm.DB, group string, fileName string) error {
	return ReadPayloadFileLineWithCallBack(context.Background(), fileName, func(s string, rawLen int64, hitCount int64) error {
		return CreateOrUpdatePayload(db, s, group, "", hitCount, true)
	})
}

func ReadPayloadFileLineWithCallBack(ctx context.Context, fileName string, handler func(line string, rawLen int64, hitCount int64) error) error {
	fd, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer fd.Close()
	reader := bufio.NewReader(fd)

	firstLine := true
	isCSV := strings.HasSuffix(fileName, ".csv")
BREAKOUT:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			lineRaw, err := reader.ReadBytes('\n')
			if err != nil && len(lineRaw) == 0 {
				break BREAKOUT
			}
			lineRawLen := int64(len(lineRaw))
			line := string(lineRaw)
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
					if err := handler(p, lineRawLen, hitCount); err != nil {
						return err
					}
				}
			} else {
				line = strconv.Quote(strings.TrimRightFunc(line, TrimWhitespaceExceptSpace))
				if err := handler(line, lineRawLen, hitCount); err != nil {
					return err
				}
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
	return ReadQuotedLinesWithCallBack(data, func(s string, rawLen int64) error {
		return CreateOrUpdatePayload(db, s, group, "", 0, false)
	})
}

func ReadQuotedLinesWithCallBack(data string, handler func(line string, rawLen int64) error) error {
	r := bufio.NewReader(strings.NewReader(data))
	for {
		lineRaw, err := r.ReadBytes('\n')
		if err != nil && len(lineRaw) == 0 {
			break
		}
		lineRawLen := int64(len(lineRaw))
		lineRaw = bytes.TrimRightFunc(lineRaw, TrimWhitespaceExceptSpace)
		line := strconv.Quote(string(lineRaw))
		if err := handler(line, lineRawLen); err != nil {
			return err
		}
	}
	return nil
}

func GetPayloadById(db *gorm.DB, id int64) (*schema.Payload, error) {
	var req schema.Payload
	if db := db.Model(&schema.Payload{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Payload failed: %s", db.Error)
	}

	return &req, nil
}

func GetPayloadsByGroup(db *gorm.DB, group string) ([]*schema.Payload, error) {
	var reqs []*schema.Payload
	if db := db.Model(&schema.Payload{}).Where("`group` = ?", group).Find(&reqs); db.Error != nil {
		return nil, utils.Errorf("get Payload failed: %s", db.Error)
	}
	return reqs, nil
}

func GetPayloadsByFolder(db *gorm.DB, folder string) ([]*schema.Payload, error) {
	var reqs []*schema.Payload
	if db := db.Model(&schema.Payload{}).Where("`folder` = ?", folder).Find(&reqs); db.Error != nil {
		return nil, utils.Errorf("get Payload by folder failed: %s", db.Error)
	}
	return reqs, nil
}

func SetGroupInEnd(db *gorm.DB, group string) error {
	var groups []string
	if err := db.Model(&schema.Payload{}).Select("`group`").Group("`group`").Pluck("`group`", &groups).Error; err != nil {
		return err
	}
	// 更新group_index
	if err := db.Model(&schema.Payload{}).Where("`group` = ?", group).Update("group_index", len(groups)+1).Error; err != nil {
		return err
	}
	return nil
}

func GetPayloadFirst(db *gorm.DB, group string) (*schema.Payload, error) {
	var req schema.Payload
	if db := db.Model(&schema.Payload{}).Where("`group` = ?", group).First(&req); db.Error != nil {
		return nil, utils.Wrapf(db.Error, "get Payload by group %s failed", group)
	} else {
		return &req, nil
	}
}

func GetPayloadGroupFileName(db *gorm.DB, group string) (string, error) {
	if payload, err := GetPayloadFirst(db, group); err != nil {
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
	if db := db.Model(&schema.Payload{}).Where("`group` = ?", group).Count(&i); db.Error != nil {
		return 0
	}
	return i
}

func DeletePayloadByID(db *gorm.DB, id int64) error {
	if err := db.Unscoped().Delete(&schema.Payload{}, id).Error; err != nil {
		return err
	} else {
		return nil
	}
}

func DeletePayloadByIDs(db *gorm.DB, ids []int64) error {
	if err := db.Unscoped().Delete(&schema.Payload{}, ids).Error; err != nil {
		return err
	} else {
		return nil
	}
}

func DeletePayloadByGroup(db *gorm.DB, group string) error {
	if db := db.Model(&schema.Payload{}).Where(
		"`group` = ?", group,
	).Unscoped().Delete(&schema.Payload{}); db.Error != nil {
		return db.Error
	} else {
		return nil
	}
}

func DeletePayloadByFolder(db *gorm.DB, folder string) error {
	if db := db.Model(&schema.Payload{}).Where(
		"`folder` = ?", folder,
	).Unscoped().Delete(&schema.Payload{}); db.Error != nil {
		return db.Error
	} else {
		return nil
	}
}

func RenamePayloadFolder(db *gorm.DB, folder, newFolder string) error {
	return db.Model(&schema.Payload{}).Where("`folder` = ?", folder).Update("folder", newFolder).Error
}

func RenamePayloadGroup(db *gorm.DB, oldGroup, newGroup string) error {
	return db.Model(&schema.Payload{}).Where("`group` = ?", oldGroup).Update("group", newGroup).Error
}

func CopyPayloads(db *gorm.DB, payloads []*schema.Payload, group, folder string) error {
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

func MovePayloads(db *gorm.DB, payloads []*schema.Payload, group, folder string) error {
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
	db = db.Model(&schema.Payload{})
	// 查找或创建一个新的记录
	payload := schema.Payload{
		Folder:     &folder,
		Group:      group,
		GroupIndex: &group_index,
	}
	db = db.FirstOrCreate(&payload, schema.Payload{Folder: &folder, Group: group})

	// 如果创建失败，返回错误
	if db.Error != nil {
		return utils.Wrap(db.Error, "create or find payload failed")
	}

	// 更新group_index
	if err := db.Model(&schema.Payload{}).Where("`folder` = ?", folder).Where("`group` = ?", group).Update("group_index", group_index).Error; err != nil {
		return err
	}
	return nil
}

func UpdatePayloadGroup(db *gorm.DB, group, folder string, group_index int64) error {
	return db.
		Model(&schema.Payload{}).
		Where("`group` = ?", group).
		Updates(map[string]any{
			"folder":      folder,
			"group_index": group_index,
		}).Error
}

func UpdatePayload(db *gorm.DB, id int, payload *schema.Payload) error {
	db = db.Model(&schema.Payload{}).Where("`id` = ?", id).Updates(map[string]any{"group": payload.Group, "folder": payload.Folder, "group_index": payload.GroupIndex, "content": payload.Content, "hit_count": payload.HitCount, "is_file": payload.IsFile, "hash": payload.Hash})
	if err := db.Error; err != nil {
		return utils.Wrap(err, "update payload error")
	}
	return nil
}

func UpdatePayloadColumns(db *gorm.DB, id int, m map[string]any) error {
	if err := db.Model(&schema.Payload{}).Where("`id` = ?", id).Updates(m).Error; err != nil {
		return utils.Wrap(err, "update payload error")
	}
	return nil
}

func PayloadGroups(db *gorm.DB, search ...string) []string {
	if len(search) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "`group`", search)
	}
	rows, err := db.Model(&schema.Payload{}).Select("distinct `group`").Rows()
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

func QueryPayloadWithoutPaging(db *gorm.DB, folder, group, keyword string) ([]*schema.Payload, error) {
	db = db.Model(&schema.Payload{})
	if group != "" {
		db = db.Where("`group` = ?", group)
	}
	if folder != "" {
		db = db.Where("`folder` = ?", folder)
	}
	if keyword != "" {
		db = db.Where("`content` = ?", keyword)
	}

	var ret []*schema.Payload
	db = db.Find(&ret)
	if db.Error != nil {
		return nil, utils.Errorf("query payload failed: %s", db.Error)
	}
	return ret, nil
}

func QueryPayload(db *gorm.DB, folder, group, keyword string, paging *Paging) (*bizhelper.Paginator, []*schema.Payload, error) {
	db = db.Model(&schema.Payload{})
	db = bizhelper.QueryOrder(db, paging.OrderBy, paging.Order)
	db = bizhelper.ExactQueryString(db, "`folder`", folder)
	db = bizhelper.ExactQueryString(db, "`group`", group)
	// db = bizhelper.QueryByBool(db, "`is_file`", false)
	db = bizhelper.FuzzQueryLike(db, "content", keyword)
	var ret []*schema.Payload
	pag, db := bizhelper.Paging(db, paging.Page, paging.Limit, &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, ret, nil
}

func YieldPayloads(db *gorm.DB, ctx context.Context) chan *schema.Payload {
	return bizhelper.YieldModel[*schema.Payload](ctx, db)
}

func GetAllPayloadGroupName(db *gorm.DB) ([]string, error) {
	var groups []string
	if db := db.Model(&schema.Payload{}).Pluck("DISTINCT(`group`)", &groups); db.Error != nil {
		return nil, db.Error
	}
	groups = lo.Filter(groups, func(s string, _ int) bool {
		return !strings.HasSuffix(s, "///empty")
	})
	return groups, nil
}

func GetPayload(db *gorm.DB, groups []string) ([]*schema.Payload, error) {
	var req []*schema.Payload
	if db := db.Model(&schema.Payload{}).Where("`group` in (?) ", groups).Scan(&req); db.Error != nil {
		return nil, utils.Wrapf(db.Error, "get Payload by groups  failed: %s", db.Error)
	} else {
		return req, nil
	}
}
