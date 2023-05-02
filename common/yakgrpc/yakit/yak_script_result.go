package yakit

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/bizhelper"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

type ExecResult struct {
	gorm.Model

	YakScriptName string `json:"yak_script_name" gorm:"index"`
	Raw           string `json:"raw"`
}

type TagAndTypeValue struct {
	Value string
	Count int
}

func (e *ExecResult) ToGRPCModel() *ypb.ExecResult {
	var res ypb.ExecResult
	err := json.Unmarshal([]byte(e.Raw), &res)
	if err != nil {
		return nil
	}
	res.Id = int64(e.ID)
	return &res
}

func SaveExecResult(db *gorm.DB, yakScriptName string, r *ypb.ExecResult) error {
	if r == nil {
		return utils.Errorf("empty exec result")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}

	db.Save(&ExecResult{
		YakScriptName: yakScriptName,
		Raw:           string(raw),
	})
	return nil
}

func CreateOrUpdateExecResult(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&ExecResult{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&ExecResult{}); db.Error != nil {
		return utils.Errorf("create/update ExecResult failed: %s", db.Error)
	}

	return nil
}

func GetExecResult(db *gorm.DB, id int64) (*ExecResult, error) {
	var req ExecResult
	if db := db.Model(&ExecResult{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ExecResult failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteExecResultByID(db *gorm.DB, id int64) error {
	if db := db.Model(&ExecResult{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&ExecResult{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteExecResultByYakScriptName(db *gorm.DB, name string) error {
	if db := db.Model(&ExecResult{}).Where(
		"yak_script_name = ?", name,
	).Unscoped().Delete(&ExecResult{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryExecResult(db *gorm.DB, params *ypb.QueryYakScriptExecResultRequest) (*bizhelper.Paginator, []*ExecResult, error) {
	if params == nil {
		params = &ypb.QueryYakScriptExecResultRequest{}
	}

	db = db.Model(&YakScript{}) //.Debug()

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
	db = bizhelper.ExactQueryString(db, "yak_script_name", params.YakScriptName)

	var ret []*ExecResult
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func DeleteExecResult(db *gorm.DB) error {
	if db = db.Model(&ExecResult{}).Where(
		"true",
	).Unscoped().Delete(&ExecResult{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func YakScriptTags(db *gorm.DB, where string, havingWhere string) (req []*TagAndTypeValue, err error) {
	sqlWhere := `SELECT DISTINCT (LOWER(value)) as value, count(t.id) as count
			from (WITH RECURSIVE split(value, str) AS (
				SELECT null, tags || ','
				from yak_scripts WHERE (tags LIKE '%') ` + where +
		`UNION ALL
				SELECT substr(str, 0, instr(str, ',')),
					   substr(str, instr(str, ',') + 1)
				FROM split
				WHERE str != ''
			)
	      	SELECT DISTINCT value
	      	FROM split
	      	WHERE value is not NULL
	        	and value != '')
	         	join yak_scripts t on ( tags LIKE '%' || value || '%') ` + where + ` where value != '' and value != 'null'
			group by value ` + havingWhere + ` order by count desc;`
	db = db.Raw(sqlWhere)
	db = db.Scan(&req)
	if db.Error != nil {
		return nil, utils.Errorf("tag group rows failed: %s", db.Error)
	}

	return req, nil
}

func YakScriptType(db *gorm.DB) (req []*TagAndTypeValue, err error) {
	db = db.Raw(`SELECT count(*) as count, type as value FROM yak_scripts GROUP BY type order by count desc;`)
	db = db.Scan(&req)
	if db.Error != nil {
		return nil, utils.Errorf("type group rows failed: %s", db.Error)
	}

	return req, nil
}
