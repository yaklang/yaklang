package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
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
	_, isJson := utils.IsJSON(raw)
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
		IsJson:               isJson,
		IsProtobuf:           utils.IsProtobuf([]byte(raw)),
	}
}

func (f *WebsocketFlow) CalcHash() string {
	return utils.CalcSha1(f.WebsocketRequestHash, f.FrameIndex)
}

func (f *WebsocketFlow) BeforeSave() error {
	f.Hash = f.CalcHash()
	return nil
}
