package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"strings"
)

type WebsocketFlow struct {
	gorm.Model

	// WebsocketFlow 过来的应该有 WebsocketHash
	WebsocketRequestHash string `json:"websocket_request_hash" gorm:"index"`

	FrameIndex  int    `json:"frame_index" gorm:"index"`
	FromServer  bool   `json:"from_server"`
	QuotedData  string `json:"quoted_data"`
	MessageType string `json:"message_type"`
	Tags        string `json:"tags"`

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

// 颜色与 Tag API
func (f *WebsocketFlow) AddTag(appendTags ...string) {
	existed := utils.PrettifyListFromStringSplited(f.Tags, "|")
	existedCount := len(existed)
	extLen := len(appendTags)
	tags := make([]string, existedCount+extLen)
	copy(tags, existed)
	for i := 0; i < extLen; i++ {
		tags[i+existedCount] = appendTags[i]
	}
	f.Tags = strings.Join(utils.RemoveRepeatStringSlice(tags), "|")
}

func (f *WebsocketFlow) AddTagToFirst(appendTags ...string) {
	existed := utils.PrettifyListFromStringSplited(f.Tags, "|")
	f.Tags = strings.Join(utils.RemoveRepeatStringSlice(append(appendTags, existed...)), "|")
}

func (f *WebsocketFlow) HasColor(color string) bool {
	return utils.StringArrayContains(utils.PrettifyListFromStringSplited(f.Tags, "|"), color)
}

func (f *WebsocketFlow) Red() {
	if f.HasColor(FLOW_COLOR_RED) {
		return
	}
	f.AddTag(FLOW_COLOR_RED)
}

func (f *WebsocketFlow) Green() {
	if f.HasColor(FLOW_COLOR_GREEN) {
		return
	}
	f.AddTag(FLOW_COLOR_GREEN)
}

func (f *WebsocketFlow) Blue() {
	if f.HasColor(FLOW_COLOR_BLUE) {
		return
	}
	f.AddTag(FLOW_COLOR_BLUE)
}

func (f *WebsocketFlow) Yellow() {
	if f.HasColor(FLOW_COLOR_YELLOW) {
		return
	}
	f.AddTag(FLOW_COLOR_YELLOW)
}

func (f *WebsocketFlow) Orange() {
	if f.HasColor(FLOW_COLOR_ORANGE) {
		return
	}
	f.AddTag(FLOW_COLOR_ORANGE)
}

func (f *WebsocketFlow) Purple() {
	if f.HasColor(FLOW_COLOR_PURPLE) {
		return
	}
	f.AddTag(FLOW_COLOR_PURPLE)
}

func (f *WebsocketFlow) Cyan() {
	if f.HasColor(FLOW_COLOR_CYAN) {
		return
	}
	f.AddTag(FLOW_COLOR_CYAN)
}

func (f *WebsocketFlow) Grey() {
	if f.HasColor(FLOW_COLOR_GREY) {
		return
	}
	f.AddTag(FLOW_COLOR_GREY)
}
