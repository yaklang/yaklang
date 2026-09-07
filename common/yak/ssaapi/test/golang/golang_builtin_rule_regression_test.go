package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func loadGolangBuiltinRule(t *testing.T, path string) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent(path)
	if !ok {
		t.Skipf("%s 不在当前构建的 embed FS 中，跳过测试", path)
	}
	require.NotEmpty(t, content, "%s 内容为空", path)
	return content
}

func runGolangBuiltinRule(t *testing.T, ruleContent, filename, code string) int {
	t.Helper()

	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, code)

	total := 0
	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		result, err := programs.SyntaxFlowWithError(ruleContent)
		require.NoError(t, err)

		for _, name := range result.GetAlertVariables() {
			total += len(result.GetValues(name))
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

	return total
}

func TestGolangFileUploadRule_Positive_GenericUploadHelper(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "golang/cwe-434-file-upload/golang-file-upload.sf")

	total := runGolangBuiltinRule(t, rule, "upload_positive.go", `
package demo

import "mime/multipart"

type Result struct{}

func Upload(name string, contentType string, payload any) (*Result, error) {
	return nil, nil
}

func save(h *multipart.FileHeader) (*Result, error) {
	return Upload(h.Filename, "", nil)
}
`)

	assert.Greater(t, total, 0, "未验证的上传文件名流入上传sink应触发告警")
}

func TestGolangFileUploadRule_Negative_NonUploadFileWrite(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "golang/cwe-434-file-upload/golang-file-upload.sf")

	total := runGolangBuiltinRule(t, rule, "upload_negative_non_upload.go", `
package demo

import "os"

type Controller struct{}

func (c *Controller) GetString(key string) string {
	return ""
}

func (c *Controller) createDir(parent string) error {
	title := c.GetString("name")
	return os.MkdirAll(parent+"/"+title, 0o777)
}
`)

	assert.Equal(t, 0, total, "普通用户输入流向文件系统操作不应被文件上传规则误报")
}

func TestGolangFileUploadRule_Positive_DirectWriteSink(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "golang/cwe-434-file-upload/golang-file-upload.sf")

	total := runGolangBuiltinRule(t, rule, "upload_direct_write_positive.go", `
package demo

import (
	"mime/multipart"
	"os"
)

func save(h *multipart.FileHeader) error {
	dst, err := os.Create(h.Filename)
	if err != nil {
		return err
	}
	return dst.Close()
}
`)

	assert.Greater(t, total, 0, "未验证的上传文件名直接进入文件写入sink应触发告警")
}

func TestGolangFileUploadRule_Negative_SanitizedWriteSink(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "golang/cwe-434-file-upload/golang-file-upload.sf")

	total := runGolangBuiltinRule(t, rule, "upload_sanitized_write_negative.go", `
package demo

import (
	"mime/multipart"
	"os"
	"path/filepath"
)

func save(h *multipart.FileHeader) error {
	dst, err := os.Create(filepath.Base(h.Filename))
	if err != nil {
		return err
	}
	return dst.Close()
}
`)

	assert.Equal(t, 0, total, "经过filepath.Base约束的上传文件名不应触发告警")
}

func TestGolangBeegoORMFilterRule_Positive(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "golang/cwe-89-sql-injection/golang-beego-orm-filter-sql.sf")

	total := runGolangBuiltinRule(t, rule, "beego_filter_positive.go", `
package demo

import (
	beego "github.com/beego/beego/v2/server/web"
	"github.com/beego/beego/v2/client/orm"
)

type Controller struct{}
type ProjectController struct {
	beego.Controller
}

func (c *ProjectController) handle() {
	projid := c.GetString("projid")
	var paths []string
	o := orm.NewOrm()
	qs := o.QueryTable("casbin_rule")
	_, _ = qs.Filter("PType", "p").Filter("v1__contains", "/"+projid+"/").All(&paths)
}
`)

	assert.Greater(t, total, 0, "外部输入拼接进Beego ORM Filter值应触发告警")
}

func TestGolangBeegoORMFilterRule_Negative_ExactID(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "golang/cwe-89-sql-injection/golang-beego-orm-filter-sql.sf")

	total := runGolangBuiltinRule(t, rule, "beego_filter_negative.go", `
package demo

import (
	"strconv"

	beego "github.com/beego/beego/v2/server/web"
	"github.com/beego/beego/v2/client/orm"
)

type ProjectController struct {
	beego.Controller
}

func (c *ProjectController) handle() {
	projidStr := c.GetString("projid")
	projid, err := strconv.ParseInt(projidStr, 10, 64)
	if err != nil {
		return
	}

	var paths []string
	o := orm.NewOrm()
	qs := o.QueryTable("casbin_rule")
	_, _ = qs.Filter("project_id", projid).All(&paths)
}
`)

	assert.Equal(t, 0, total, "未做字符串拼接的精确ORM过滤不应命中这条规则")
}

func TestGolangPathJoinParameterRule_Positive(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "golang/cwe-73-unfiltered-file-or-path/golang-filepath-join-parameter.sf")

	total := runGolangBuiltinRule(t, rule, "path_join_positive.go", `
package demo

import (
	"os"
	"path/filepath"
)

func DeleteFile(path string, filename string) error {
	backupFilePath := filepath.Join(path, filename)
	if _, err := os.Stat(backupFilePath); err != nil {
		return err
	}
	return os.Remove(backupFilePath)
}
`)

	assert.Greater(t, total, 0, "外部参数经filepath.Join后直接进入文件操作应触发告警")
}

func TestGolangPathJoinParameterRule_Negative_CleanAndPrefixCheck(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "golang/cwe-73-unfiltered-file-or-path/golang-filepath-join-parameter.sf")

	total := runGolangBuiltinRule(t, rule, "path_join_negative.go", `
package demo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func DeleteFile(baseDir string, filename string) error {
	cleanPath := filepath.Clean(filepath.Join(baseDir, filename))
	if !strings.HasPrefix(cleanPath, baseDir) {
		return errors.New("invalid path")
	}
	if _, err := os.Stat(cleanPath); err != nil {
		return err
	}
	return os.Remove(cleanPath)
}
`)

	assert.Equal(t, 0, total, "有明确Clean和前缀边界检查时不应命中")
}

func TestGolangGoldmarkAttributeXSSRule_Positive(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "golang/cwe-79-xss/golang-goldmark-attribute-xss.sf")

	total := runGolangBuiltinRule(t, rule, "goldmark_xss_positive.go", `
package demo

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type hookedRenderer struct {
	html.Config
}

func (r *hookedRenderer) renderLinkDefault(w util.BufWriter, node ast.Node) {
	n := node.(*ast.Link)
	if n.Title != nil {
		r.Writer.Write(w, n.Title)
	}
}
`)

	assert.Greater(t, total, 0, "未转义写入Goldmark标题属性应触发XSS告警")
}

func TestGolangGoldmarkAttributeXSSRule_Negative_EscapedTitle(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "golang/cwe-79-xss/golang-goldmark-attribute-xss.sf")

	total := runGolangBuiltinRule(t, rule, "goldmark_xss_negative.go", `
package demo

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type hookedRenderer struct {
	html.Config
}

func (r *hookedRenderer) renderLinkDefault(w util.BufWriter, node ast.Node) {
	n := node.(*ast.Link)
	if n.Title != nil {
		r.Writer.Write(w, util.EscapeHTML(n.Title))
	}
}
`)

	assert.Equal(t, 0, total, "显式转义标题值后不应命中XSS告警")
}
