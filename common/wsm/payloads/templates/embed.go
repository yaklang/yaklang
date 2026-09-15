package templates

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"encoding/json"
	"fmt"
	"sync"

	//go:generate gzip-embed -cache --source ./static --gz static.tar.gz --xor-key yaklang-wsm-v1 --no-embed
)

const templatesXorKey = "yaklang-wsm-v1"

//go:embed static
var templatesRawFS embed.FS

var templatesRawFSIns = filesys.NewEmbedSubFS(templatesRawFS, "static")

type WebshellTemplates struct {
	JspDefaultHeader        string `json:"jsp_default_header"`
	JspDefaultDecode        string `json:"jsp_default_decode"`
	JspDefaultMemberDecl    string `json:"jsp_default_member_decl"`
	JspDefaultServiceDecl   string `json:"jsp_default_service_decl"`
	JspSessionServiceDecl   string `json:"jsp_session_service_decl"`
	PhpDefaultHeader        string `json:"php_default_header"`
	PhpDefaultDecode        string `json:"php_default_decode"`
	PhpDefaultMemberDecl    string `json:"php_default_member_decl"`
	PhpDefaultServiceDecl   string `json:"php_default_service_decl"`
	PhpSessionMemberDecl    string `json:"php_session_member_decl"`
	PhpSessionServiceDecl   string `json:"php_session_service_decl"`
	AspxDefaultHeader       string `json:"aspx_default_header"`
	AspxDefaultServiceDecl  string `json:"aspx_default_service_decl"`
	AspxSessionServiceDecl  string `json:"aspx_session_service_decl"`
	BehinderPhpEchoEncoder  string `json:"behinder_php_echo_encoder"`
	BehinderAspEchoEncoder  string `json:"behinder_asp_echo_encoder"`
	BehinderPhpAssertPrefix string `json:"behinder_php_assert_prefix"`
	BehinderPhpAssertSuffix string `json:"behinder_php_assert_suffix"`
}

var (
	templatesInstance *WebshellTemplates
	once              sync.Once
)

// GetTemplates 返回解码后的 webshell 模板配置（单例）。
func GetTemplates() *WebshellTemplates {
	once.Do(func() {
		raw, err := templatesRawFSIns.ReadFile("webshell_templates.json")
		if err != nil {
			panic(fmt.Sprintf("read webshell_templates.json failed: %v", err))
		}
		t := &WebshellTemplates{}
		if err := json.Unmarshal(raw, t); err != nil {
			panic(fmt.Sprintf("unmarshal webshell templates failed: %v", err))
		}
		templatesInstance = t
	})
	return templatesInstance
}
