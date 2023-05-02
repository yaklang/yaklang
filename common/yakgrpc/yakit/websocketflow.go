package yakit

import (
	"github.com/asaskevich/govalidator"
	"github.com/jinzhu/gorm"
	"strconv"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type WebsocketFlow struct {
	gorm.Model

	// HTTPFlow 过来的应该有 WebsocketHash
	WebsocketRequestHash string `json:"websocket_request_hash" gorm:"index"`

	FrameIndex  int    `json:"frame_index" gorm:"index"`
	FromServer  bool   `json:"from_server"`
	QuotedData  string `json:"quoted_data"`
	MessageType string `json:"message_type"`

	Hash string `json:"hash"`
}

func (i *WebsocketFlow) ToGRPCModel() *ypb.WebsocketFlow {
	raw, _ := strconv.Unquote(i.QuotedData)
	if len(raw) <= 0 {
		raw = i.QuotedData
	}

	length := len(raw)
	return &ypb.WebsocketFlow{
		ID:                   int64(i.ID),
		CreatedAt:            i.CreatedAt.Unix(),
		WebsocketRequestHash: i.WebsocketRequestHash,
		FrameIndex:           int64(i.FrameIndex),
		FromServer:           i.FromServer,
		MessageType:          i.MessageType,
		Data:                 []byte(raw),
		DataSizeVerbose:      utils.ByteSize(uint64(length)),
		DataLength:           int64(length),
		DataVerbose:          utils.DataVerbose(raw),
		IsJson:               govalidator.IsJSON(raw),
		IsProtobuf:           utils.IsProtobuf([]byte(raw)),
	}
}

func SaveToServerWebsocketFlow(db *gorm.DB, owner string, index int, data []byte) error {
	f := &WebsocketFlow{
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
	f := &WebsocketFlow{
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

func (f *WebsocketFlow) CalcHash() string {
	return utils.CalcSha1(f.WebsocketRequestHash, f.FrameIndex)
}

func (f *WebsocketFlow) BeforeSave() error {
	f.Hash = f.CalcHash()
	return nil
}

func CreateOrUpdateWebsocketFlow(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&WebsocketFlow{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&WebsocketFlow{}); db.Error != nil {
		return utils.Errorf("create/update WebsocketFlow failed: %s", db.Error)
	}

	return nil
}

func GetWebsocketFlow(db *gorm.DB, id int64) (*WebsocketFlow, error) {
	var req WebsocketFlow
	if db := db.Model(&WebsocketFlow{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get WebsocketFlow failed: %s", db.Error)
	}

	return &req, nil
}

func QueryWebsocketFlowByWebsocketHash(db *gorm.DB, hash string, page int, limit int) (*bizhelper.Paginator, []*WebsocketFlow, error) {
	db = db.Model(&WebsocketFlow{})

	if hash == "" {
		return nil, nil, utils.Errorf("empty hash")
	}

	db = bizhelper.ExactQueryString(db, "websocket_request_hash", hash)
	db = bizhelper.QueryOrder(db, "frame_index", "desc")

	var ret []*WebsocketFlow
	paging, db := bizhelper.Paging(db, page, limit, &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func DeleteWebsocketFlowByID(db *gorm.DB, id int64) error {
	if db := db.Model(&WebsocketFlow{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&WebsocketFlow{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteWebsocketFlowByWebsocketHash(db *gorm.DB, hash string) error {
	if db := db.Model(&WebsocketFlow{}).Where(
		"websocket_request_hash = ?", hash,
	).Unscoped().Delete(&WebsocketFlow{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteWebsocketFlowAll(db *gorm.DB) error {
	if db := db.Model(&WebsocketFlow{}).Where("true").Unscoped().Delete(&WebsocketFlow{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteWebsocketFlowsByHTTPFlowHash(db *gorm.DB, hash []string) error {
	db = db.Model(&WebsocketFlow{}).Where(
		"websocket_request_hash in (?)", hash,
	).Unscoped().Delete(&WebsocketFlow{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}
