//go:generate xorencode -input webshell_templates.json -output webshell_templates.json.enc -key yaklang-wsm-v1
package templates

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/utils/xorencoded"
)

const xorKey = "yaklang-wsm-v1"

//go:embed webshell_templates.json.enc
var templatesFS embed.FS

// WebshellTemplates 是从 XOR 编码文件中解码出来的所有 webshell 生成模板。
type WebshellTemplates struct {
	JspDefaultHeader          string `json:"jsp_default_header"`
	JspDefaultDecode          string `json:"jsp_default_decode"`
	JspDefaultMemberDecl      string `json:"jsp_default_member_decl"`
	JspDefaultServiceDecl     string `json:"jsp_default_service_decl"`
	JspSessionServiceDecl     string `json:"jsp_session_service_decl"`
	PhpDefaultHeader          string `json:"php_default_header"`
	PhpDefaultDecode          string `json:"php_default_decode"`
	PhpDefaultMemberDecl      string `json:"php_default_member_decl"`
	PhpDefaultServiceDecl     string `json:"php_default_service_decl"`
	PhpSessionMemberDecl      string `json:"php_session_member_decl"`
	PhpSessionServiceDecl     string `json:"php_session_service_decl"`
	AspxDefaultHeader         string `json:"aspx_default_header"`
	AspxDefaultServiceDecl    string `json:"aspx_default_service_decl"`
	AspxSessionServiceDecl    string `json:"aspx_session_service_decl"`
	BehinderPhpEchoEncoder    string `json:"behinder_php_echo_encoder"`
	BehinderAspEchoEncoder    string `json:"behinder_asp_echo_encoder"`
	BehinderPhpAssertPrefix   string `json:"behinder_php_assert_prefix"`
	BehinderPhpAssertSuffix   string `json:"behinder_php_assert_suffix"`
}

var (
	templatesInstance *WebshellTemplates
	once              sync.Once
)

// GetTemplates 返回解码后的 webshell 模板配置（单例）。
func GetTemplates() *WebshellTemplates {
	once.Do(func() {
		content, err := xorencoded.LoadEmbedFile(templatesFS, "webshell_templates.json.enc", []byte(xorKey))
		if err != nil {
			panic(fmt.Sprintf("load webshell templates failed: %v", err))
		}
		t := &WebshellTemplates{}
		if err := json.Unmarshal([]byte(content), t); err != nil {
			panic(fmt.Sprintf("unmarshal webshell templates failed: %v", err))
		}
		templatesInstance = t
	})
	return templatesInstance
}
