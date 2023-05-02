package yakit

import (
	"bufio"
	"context"
	"github.com/jinzhu/gorm"
	"os"
	"strconv"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
	"yaklang/common/yakgrpc/ypb"
)

type Payload struct {
	gorm.Model

	Group string `json:"group" gorm:"index"`

	// strconv Quoted
	Content string `json:"content"`

	// Hash string
	Hash string `json:"hash" gorm:"unique_index"`
}

func (p *Payload) CalcHash() string {
	return utils.CalcSha1(p.Group, p.Content)
}

func (p *Payload) BeforeSave() error {
	if p.Hash == "" {
		p.Hash = p.CalcHash()
	}
	return nil
}

type gormNoLog int

func (i gormNoLog) Print(v ...interface{}) {

}

func CreateOrUpdatePayload(db *gorm.DB, hash string, i *Payload) error {
	db = db.Model(&Payload{})
	db.SetLogger(gormNoLog(1))
	if db := db.Save(i); db.Error != nil {

	}
	//if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&Payload{}); db.Error != nil {
	//	return utils.Errorf("create/update Payload failed: %s", db.Error)
	//}

	return nil
}

func SavePayloadByFilename(db *gorm.DB, group string, fileName string) error {
	fp, err := os.Open(fileName)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(fp)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		payload := &Payload{
			Group:   group,
			Content: strconv.Quote(scanner.Text()),
		}
		payload.Hash = payload.CalcHash()
		err := CreateOrUpdatePayload(db, payload.Hash, payload)
		if err != nil {
			log.Errorf("create or update payload error: %s", err.Error())
			continue
		}
	}
	return nil
}

func SavePayload(db *gorm.DB, group string, payloads ...string) error {
	for _, p := range payloads {
		payload := &Payload{
			Group:   group,
			Content: strconv.Quote(p),
		}
		payload.Hash = payload.CalcHash()
		err := CreateOrUpdatePayload(db, payload.Hash, payload)
		if err != nil {
			log.Errorf("create or update payload error: %s", err.Error())
			continue
		}
	}
	return nil
}

func GetPayload(db *gorm.DB, id int64) (*Payload, error) {
	var req Payload
	if db := db.Model(&Payload{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Payload failed: %s", db.Error)
	}

	return &req, nil
}

func SavePayloadGroup(db *gorm.DB, group string, lists []string) error {
	for _, i := range lists {
		p := &Payload{
			Group:   group,
			Content: i,
		}
		p.Hash = p.CalcHash()
		err := CreateOrUpdatePayload(db, p.Hash, p)
		if err != nil {
			return err
		}
	}
	return nil
}

func GetPayloadCount(db *gorm.DB, group string) int64 {
	var i int64
	if db := db.Model(&Payload{}).Where("`group` = ?", group).Count(&i); db.Error != nil {
		return 0
	}
	return i
}

func DeletePayloadByID(db *gorm.DB, id int64) error {
	if db := db.Model(&Payload{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&Payload{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeletePayloadByGroup(db *gorm.DB, group string) error {
	if db := db.Model(&Payload{}).Where(
		"`group` = ?", group,
	).Unscoped().Delete(&Payload{}); db.Error != nil {
		return db.Error
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

func QueryPayload(db *gorm.DB, params *ypb.QueryPayloadRequest) (*bizhelper.Paginator, []*Payload, error) {
	if params == nil {
		return nil, nil, utils.Errorf("empty params")
	}
	db = db.Model(&Payload{}) // .Debug()
	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	p := params.Pagination
	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)
	db = bizhelper.ExactQueryString(db, "`group`", params.GetGroup())
	db = bizhelper.FuzzQueryLike(db, "content", params.GetKeyword())

	var ret []*Payload
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func YieldPayloads(db *gorm.DB, ctx context.Context) chan *Payload {
	outC := make(chan *Payload)
	go func() {
		defer close(outC)

		var page = 1
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

type SimplePort struct {
	Host string
	Port int
}

func UpdatePayload(db *gorm.DB, params *ypb.UpdatePayloadRequest) error {
	db = db.Model(&Payload{}).Where("`group` = ?", params.GetOldGroup()).Update("group", params.GetGroup())
	if db.Error != nil {
		return utils.Errorf("update Payload failed: %s", db.Error)
	}
	return nil
}
