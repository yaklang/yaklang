package tools

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/go-git/go-git/v5"
	gitClient "github.com/go-git/go-git/v5/plumbing/transport/client"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"gopkg.in/yaml.v2"
)

type templateDesc struct {
	Id   string `yaml:"id"`
	Info struct {
		Name        string `yaml:"name"`
		Author      string `yaml:"author"`
		Description string `yaml:"description"`
		Tags        string `yaml:"tags"`
	} `yaml:"info"`
	Raw       string `yaml:"-"`
	LocalPath string `yaml:"-"`
}

// AllPoC 获取本地当前已加载的全部 nuclei 模板(PoC)描述信息
// 参数:
//   - defaultDirs: 可选，指定模板所在目录，不传时使用默认模板目录
//
// 返回值:
//   - []*templateDesc: 模板描述信息列表
//   - error: 读取失败时返回错误
//
// Example:
// ```
// // 该示例为示意性用法：列出本地所有 nuclei 模板
// pocs, err = nuclei.AllPoC()
// die(err)
// println(len(pocs))
// ```
func FetchCurrentNucleiTemplates(defaultDirs ...string) ([]*templateDesc, error) {
	var templates []*templateDesc
	homeDir := consts.GetDefaultBaseHomeDir()
	var prefixDir string
	if len(defaultDirs) > 0 {
		prefixDir = utils.GetFirstExistedPath(defaultDirs...)
		if prefixDir == "" {
			return nil, utils.Errorf("cannot found existed path: %v", defaultDirs)
		}
	}
	if prefixDir == "" {
		prefixDir = filepath.Join(homeDir, "nuclei-templates")
		if !strings.HasSuffix(prefixDir, string(filepath.Separator)) {
			prefixDir += string(filepath.Separator)
		}
	}

	files, err := utils.ReadFilesRecursivelyWithLimit(
		prefixDir, 100000,
	)
	if err != nil {
		return nil, err
	}

	for _, r := range files {
		if r.IsDir {
			continue
		}

		raw, _ := ioutil.ReadFile(r.Path)
		var tempDesc templateDesc
		err = yaml.Unmarshal(raw, &tempDesc)
		if err != nil {
			continue
		}

		if tempDesc.Info.Name == "" {
			continue
		}

		if tempDesc.Id != "" {
			tempDesc.Raw = string(raw)
			templates = append(templates, &tempDesc)
			if strings.HasPrefix(r.Path, prefixDir) {
				tempDesc.LocalPath = strings.ReplaceAll(r.Path, prefixDir, "")
			} else {
				tempDesc.LocalPath = r.Path
			}
		}
	}
	return templates, nil
}

var BuildinNucleiYakScriptParam = []*ypb.YakScriptParam{
	{
		Field:        "target",
		DefaultValue: "",
		TypeVerbose:  "string",
		Required:     true,
		FieldVerbose: "扫描目标",
		Help:         "扫描目标可接受：主机名 / 主机名:端口 / IP段 / URL 等多种格式",
	},
	{
		Field:        "reverse-url",
		DefaultValue: "",
		TypeVerbose:  "string",
		FieldVerbose: "反连 URL",
		Help:         "可使用 InteractshURL 也可使用 Yakit Reverse URL",
	},
	{
		Field:        "debug",
		DefaultValue: "",
		TypeVerbose:  "boolean",
		FieldVerbose: "设置调试模式",
		Help:         "开启调试模式，调试模式将输出尽量多的调试信息",
	},
	{
		Field:        "proxy",
		DefaultValue: "",
		TypeVerbose:  "proxy",
		FieldVerbose: "HTTP 代理",
		Help:         "设置 HTTP 代理",
	},
}

// PullDatabase 从指定的 Git 仓库拉取 nuclei 模板到本地模板目录
// 参数:
//   - giturl: nuclei 模板 Git 仓库地址
//   - proxy: 可选，拉取时使用的代理地址
//
// 返回值:
//   - string: 拉取后本地模板目录路径
//   - error: 拉取失败时返回错误
//
// Example:
// ```
// // 该示例为示意性用法：从 Git 仓库拉取模板
// dir, err = nuclei.PullDatabase("https://github.com/projectdiscovery/nuclei-templates")
// die(err)
// println(dir)
// ```
func PullTemplatesFromGithub(giturl string, proxy ...string) (string, error) {
	dir := consts.GetDefaultBaseHomeDir()
	nDir := filepath.Join(dir, "nuclei-templates")

	proxy = utils.StringArrayFilterEmpty(proxy)

	if utils.GetFirstExistedPath(nDir) != "" {
		err := os.Rename(nDir, filepath.Join(dir, fmt.Sprintf("_nuclei-templates-%v", time.Now().Format(utils.DefaultTimeFormat))))
		if err != nil {
			log.Warnf("backup error: %s", err)
			//return "", utils.Errorf("backup error: %s", err)
		}
	}

	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, Proxy: http.ProxyFromEnvironment}
	if len(proxy) > 0 {
		u, err := url.Parse(proxy[0])
		if err != nil {
			return "", utils.Errorf("parse proxy[%v] failed: %s", proxy[0], err)
		}

		if !utils.MatchAnyOfSubString(u.Scheme, "http", "https", "socks5") {
			return "", utils.Errorf("proxy's schema invalid: %v", u.Scheme)
		}

		tr.Proxy = func(request *http.Request) (*url.URL, error) {
			tUrl, _ := lowhttp.ExtractURLFromHTTPRequest(request, request.URL.Scheme == "https")
			if tUrl != nil {
				log.Infof("request to %v via proxy: %v", tUrl, u.String())
			} else {
				log.Infof("request to %v via proxy: %v", request.URL.String(), u.String())
			}
			return u, nil
		}
	}

	client := &http.Client{Transport: tr, Timeout: 10 * time.Minute}
	gitClient.InstallProtocol("https", gitHttp.NewClient(client))
	gitClient.InstallProtocol("http", gitHttp.NewClient(client))

	//ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	//defer cancel()
	r, err := git.PlainCloneContext(context.Background(), nDir, false, &git.CloneOptions{
		URL: giturl, Depth: 1, Progress: os.Stdout,
	})
	if err != nil {
		return "", utils.Errorf("git clone failed: %s", err)
	}
	_ = r
	return nDir, nil
}

// UpdateDatabase 将本地 nuclei 模板目录中的 yaml PoC 加载并更新到数据库
// 参数:
//   - nucleiDir: 可选，模板目录，不传时使用默认模板目录
//
// 返回值:
//   - error: 加载失败时返回错误
//
// Example:
// ```
// // 该示例为示意性用法：将本地模板更新到数据库
// err = nuclei.UpdateDatabase()
// die(err)
// ```
func LoadYamlPoCDatabase(nucleiDir ...string) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("cannot load gorm database: %s", "empty database")
	}

	descs, err := FetchCurrentNucleiTemplates(nucleiDir...)
	if err != nil {
		log.Errorf("fetch current nuclei template: %s", err)
		return utils.Errorf("load local nuclei template failed: %s", err)
	}

	total := len(descs)
	if total <= 0 {
		return utils.Errorf("cannot load any nuclei poc... total: %v", total)
	}
	log.Infof("start to save yaml poc to database total: %v", total)
	for _, r := range descs {
		raw, _ := json.Marshal(BuildinNucleiYakScriptParam)
		y := &schema.YakScript{
			ScriptName: fmt.Sprintf("[%v]: %v", r.Id, r.Info.Name),
			Type:       "nuclei",
			Content:    r.Raw,
			Params:     strconv.Quote(string(raw)),
			Help:       r.Info.Description,
			Author:     r.Info.Author,
			Tags:       r.Info.Tags,
			FromLocal:  true,
			LocalPath:  r.LocalPath,
			IsExternal: true,
		}
		err = yakit.CreateOrUpdateYakScriptByName(db, y.ScriptName, y)
		if err != nil {
			total--
			log.Errorf("save nuclei yak script [%v] failed: %s", y.ScriptName, err)
		}
	}
	log.Infof("success for saving nuclei poc to database total: %v", total)
	return nil
}

// RemoveDatabase 从数据库中删除所有来自本地的 nuclei PoC 模板
// 返回值:
//   - error: 删除失败时返回错误
//
// Example:
// ```
// // 该示例为示意性用法：清空数据库中的本地 nuclei 模板
// err = nuclei.RemoveDatabase()
// die(err)
// ```
func RemovePoCDatabase() error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("cannot fetch database: %s", db.Error)
	}

	if db := db.Model(&schema.YakScript{}).Where(
		"(type = ?) AND (from_local = ?)",
		"nuclei", true).Unscoped().Delete(&schema.YakScript{}); db.Error != nil {
		return db.Error
	}

	return nil
}

// UpdatePoC 从默认的 nuclei 模板仓库拉取最新模板并更新到本地数据库
// 参数:
//   - proxy: 可选，拉取时使用的代理地址
//
// Example:
// ```
// // 该示例为示意性用法：更新 nuclei 模板库
// nuclei.UpdatePoC()
// ```
func UpdatePoC(proxy ...string) {
	UpdatePoCWithUrl("https://github.com/projectdiscovery/nuclei-templates", proxy...)
}

func UpdatePoCWithUrl(u string, proxy ...string) {
	dir, err := PullTemplatesFromGithub(u, proxy...)
	if err != nil {
		log.Errorf("pull nuclei templates failed: %s", err)
		return
	}
	err = LoadYamlPoCDatabase(dir)
	if err != nil {
		log.Errorf("load nuclei templates failed: %s", err)
	}
}

var NucleiOperationsExports = map[string]interface{}{
	"UpdatePoC":      UpdatePoC,
	"PullDatabase":   PullTemplatesFromGithub,
	"UpdateDatabase": LoadYamlPoCDatabase,
	"RemoveDatabase": RemovePoCDatabase,
	"AllPoC":         FetchCurrentNucleiTemplates,
	"PocVulToRisk":   PocVulToRisk,
	"GetPoCDir":      consts.GetNucleiTemplatesDir,
}
