package schema

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type WebShell struct {
	gorm.Model
	Url string `json:"url" gorm:"index" `
	// pass=payload
	Pass string `json:"pass"`
	// 加密密钥
	SecretKey string `json:"secret_key" gorm:"not null"`
	// 加密模式
	EncryptedMode string `json:"enc_mode" gorm:"column:enc_mode"`
	// 字符集编码
	Charset string `json:"charset" gorm:"default:'UTF-8'"`
	// 冰蝎还是哥斯拉,或者是其他
	ShellType string `json:"shell_type"`
	// 脚本语言
	ShellScript      string `json:"shell_script"`
	Headers          string `json:"headers" gorm:"type:json"`
	Posts            string `json:"posts" gorm:"type:json"`
	Status           bool   `json:"status"`
	Tag              string `json:"tag"`
	Proxy            string `json:"proxy"`
	Remark           string `json:"remark"`
	Hash             string `json:"hash"`
	PacketCodecName  string `json:"packet_codec_name"`
	PayloadCodecName string `json:"payload_codec_name"`
	Os               string `json:"os"`         //操作系统
	Timeout          int64  `json:"timeout"`    //超时时间
	Retry            int64  `json:"retry"`      //重连次数
	BlockSize        int64  `json:"block_size"` //分块大小
	MaxSize          int64  `json:"max_size"`   //上传的最大数量
	IsSession        bool   `json:"is_session"` //是否是session类型
}

func (w *WebShell) CalcHash() string {
	return utils.CalcSha1(w.Url)
}

func (w *WebShell) BeforeSave() error {
	if w.Url == "" {
		return utils.Error("webshell url is empty")
	}
	if w.ShellType == "" {
		return utils.Error("webshell shell type is empty")
	}
	//if w.SecretKey == "" {
	//	return utils.Error("webshell secret key is empty")
	//}
	if w.ShellScript == "" {
		return utils.Error("webshell shell script is empty")
	}
	w.Hash = w.CalcHash()
	return nil
}

func (w *WebShell) ToGRPCModel() *ypb.WebShell {
	headers := make(map[string]string)
	posts := make(map[string]string)
	if w.Headers != "" {
		err := json.Unmarshal([]byte(w.Headers), &headers)
		if err != nil {
			return nil
		}
	}
	if w.Posts != "" {
		if err := json.Unmarshal([]byte(w.Posts), &posts); err != nil {
			return nil
		}
	}
	return &ypb.WebShell{
		Id:               int64(w.ID),
		Url:              w.Url,
		Pass:             w.Pass,
		SecretKey:        w.SecretKey,
		EncMode:          w.EncryptedMode,
		Charset:          w.Charset,
		ShellType:        w.ShellType,
		ShellScript:      w.ShellScript,
		Status:           w.Status,
		Tag:              w.Tag,
		Remark:           w.Remark,
		Headers:          headers,
		Posts:            posts,
		Proxy:            w.Proxy,
		CreatedAt:        w.CreatedAt.Unix(),
		UpdatedAt:        w.UpdatedAt.Unix(),
		PayloadCodecName: w.PayloadCodecName,
		PacketCodecName:  w.PacketCodecName,
		ShellOptions: &ypb.ShellOptions{
			RetryCount: w.Retry,
			Timeout:    w.Timeout,
			BlockSize:  w.BlockSize,
			MaxSize:    w.MaxSize,
			IsSession:  w.IsSession,
		},
	}
}
