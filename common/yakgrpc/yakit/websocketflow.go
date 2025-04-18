package yakit

import (
	"context"
	"strconv"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func SaveToServerWebsocketFlow(db *gorm.DB, owner string, index int, data []byte) error {
	f := &schema.WebsocketFlow{
		WebsocketRequestHash: owner,
		FrameIndex:           index,
		FromServer:           false,
		QuotedData:           strconv.Quote(string(data)),
		MessageType:          "text",
	}
	f.Hash = f.CalcHash()
	return CreateOrUpdateWebsocketFlow(db, f.Hash, map[string]interface{}{
		"frame_index":            index,
		"from_server":            false,
		"websocket_request_hash": owner,
		"quoted_data":            strconv.Quote(string(data)),
		"message_type":           "text",
	})
}

func SaveFromServerWebsocketFlow(db *gorm.DB, owner string, index int, data []byte) error {
	f := &schema.WebsocketFlow{
		WebsocketRequestHash: owner,
		FrameIndex:           index,
		FromServer:           true,
		QuotedData:           strconv.Quote(string(data)),
		MessageType:          "text",
	}
	f.Hash = f.CalcHash()
	return CreateOrUpdateWebsocketFlow(db, f.Hash, map[string]interface{}{
		"frame_index":            index,
		"from_server":            true,
		"websocket_request_hash": owner,
		"quoted_data":            strconv.Quote(string(data)),
		"message_type":           "text",
	})
}

func BuildWebsocketFlow(fromServer bool, owner string, index int, data []byte) *schema.WebsocketFlow {
	return &schema.WebsocketFlow{
		WebsocketRequestHash: owner,
		FrameIndex:           index,
		FromServer:           fromServer,
		QuotedData:           strconv.Quote(string(data)),
		MessageType:          "text",
	}
}

func SaveWebsocketFlowEx(db *gorm.DB, wsFlow *schema.WebsocketFlow, finishHandler ...func(error)) error {
	wsFlow.Hash = wsFlow.CalcHash()
	return CreateOrUpdateWebsocketFlowEx(db, wsFlow.Hash, map[string]interface{}{
		"frame_index":            wsFlow.FrameIndex,
		"from_server":            wsFlow.FromServer,
		"websocket_request_hash": wsFlow.WebsocketRequestHash,
		"quoted_data":            wsFlow.QuotedData,
		"message_type":           "text",
		"tags":                   wsFlow.Tags,
	}, finishHandler...)
}

func CreateOrUpdateWebsocketFlowEx(db *gorm.DB, hash string, i interface{}, finishHandler ...func(error)) error {
	if consts.GLOBAL_DB_SAVE_SYNC.IsSet() {
		return CreateOrUpdateWebsocketFlow(consts.GetGormProjectDatabase(), hash, i)
	} else {
		DBSaveAsyncChannel <- func(db *gorm.DB) error {
			err := CreateOrUpdateWebsocketFlow(db, hash, i)
			for _, h := range finishHandler {
				h(err)
			}
			return err
		}
		return nil
	}
}

func CreateOrUpdateWebsocketFlow(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.WebsocketFlow{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.WebsocketFlow{}); db.Error != nil {
		return utils.Errorf("create/update WebsocketFlow failed: %s", db.Error)
	}

	return nil
}

func GetWebsocketFlow(db *gorm.DB, id int64) (*schema.WebsocketFlow, error) {
	var req schema.WebsocketFlow
	if db := db.Model(&schema.WebsocketFlow{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get WebsocketFlow failed: %s", db.Error)
	}

	return &req, nil
}

func SearchWebsocketFlow(keyword string) int {
	db := consts.GetGormProjectDatabase()
	var count int
	db.Model(&schema.WebsocketFlow{}).Where(
		"quoted_data like ?",
		"%"+keyword+"%",
	).Count(&count)
	return count
}

func QueryWebsocketFlowByWebsocketHash(db *gorm.DB, hash string, page int, limit int) (*bizhelper.Paginator, []*schema.WebsocketFlow, error) {
	db = db.Model(&schema.WebsocketFlow{})

	if hash == "" {
		return nil, nil, utils.Errorf("empty hash")
	}

	db = bizhelper.ExactQueryString(db, "websocket_request_hash", hash)
	db = bizhelper.QueryOrder(db, "frame_index", "desc")

	var ret []*schema.WebsocketFlow
	paging, db := bizhelper.Paging(db, page, limit, &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func QueryAllWebsocketFlowByWebsocketHash(db *gorm.DB, hash string) ([]*schema.WebsocketFlow, error) {
	db = db.Model(&schema.WebsocketFlow{})
	if hash == "" {
		return nil, utils.Errorf("empty hash")
	}
	var ret []*schema.WebsocketFlow
	db = bizhelper.ExactQueryString(db, "websocket_request_hash", hash)
	db = db.Find(&ret)
	if db.Error != nil {
		return nil, utils.Errorf("query websocket failed: %s", db.Error)
	}
	return ret, nil
}

func DeleteWebsocketFlowByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.WebsocketFlow{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.WebsocketFlow{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteWebsocketFlowByWebsocketHash(db *gorm.DB, hash string) error {
	if db := db.Model(&schema.WebsocketFlow{}).Where(
		"websocket_request_hash = ?", hash,
	).Unscoped().Delete(&schema.WebsocketFlow{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteWebsocketFlowAll(db *gorm.DB) error {
	if db := db.Model(&schema.WebsocketFlow{}).Where("true").Unscoped().Delete(&schema.WebsocketFlow{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DropWebsocketFlowTable(db *gorm.DB) {
	db.DropTableIfExists(&schema.WebsocketFlow{})
	if db := db.Exec(`UPDATE SQLITE_SEQUENCE SET SEQ=0 WHERE NAME='websocket_flows';`); db.Error != nil {
		log.Errorf("update sqlite sequence failed: %s", db.Error)
	}
	db.AutoMigrate(&schema.WebsocketFlow{})
}

func DeleteWebsocketFlowsByHTTPFlowHashList(db *gorm.DB, hash []string) error {
	db = db.Model(&schema.WebsocketFlow{}).Where(
		"websocket_request_hash in (?)", hash,
	).Unscoped().Delete(&schema.WebsocketFlow{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteWebsocketFlowsByHTTPFlowHash(db *gorm.DB, hash string) error {
	if db := db.Model(&schema.WebsocketFlow{}).Where(
		"websocket_request_hash = ?", hash,
	).Unscoped().Delete(&schema.WebsocketFlow{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func BatchWebsocketFlows(db *gorm.DB, ctx context.Context) chan *schema.WebsocketFlow {
	return bizhelper.YieldModel[*schema.WebsocketFlow](ctx, db)
}
