package mail

// Exports 汇总邮件库的全部能力，供 yak 脚本以 `mail.` 前缀调用。
// 注册点：common/yak/script_engine.go 的 yaklang.Import("mail", Exports)
//
// 三类能力：
//   - 解析：Parse / ParseFile / DecodeHeader / DecodeQP / ExtractURLs（钓鱼研判核心）
//   - 发送：Send + functional options（Server/SSL/STARTTLS/From/To/HTML/Attach ...）
//   - 收件：Fetch / FetchList + functional options（POP3Server/POP3SSL/MessageID ...）
var Exports = map[string]interface{}{
	// --- 解析 ---
	"Parse":        Parse,
	"ParseFile":    ParseFile,
	"DecodeHeader": DecodeHeader,
	"DecodeQP":     DecodeQP,
	"ExtractURLs":  ExtractURLs,

	// --- 发送 ---
	"Send":       Send,
	"server":     Server,
	"username":   Username,
	"password":   Password,
	"ssl":        SSL,
	"starttls":   STARTTLS,
	"notls":      NoTLS,
	"skipVerify": SkipVerify,
	"authMethod": AuthMethod,
	"from":       From,
	"to":         To,
	"cc":         Cc,
	"bcc":        Bcc,
	"subject":    Subject,
	"text":       Text,
	"html":       HTML,
	"attach":     Attach,
	"header":     Header,

	// --- 收件（POP3）---
	"Fetch":         Fetch,
	"FetchList":     FetchList,
	"pop3Server":    POP3Server,
	"pop3SSL":       POP3SSL,
	"pop3StartTLS":  POP3StartTLS,
	"pop3SkipVerify": POP3SkipVerify,
	"pop3Username":  POP3Username,
	"pop3Password":  POP3Password,
	"messageID":     MessageID,
}
