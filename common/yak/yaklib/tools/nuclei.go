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
		prefixDir := filepath.Join(homeDir, "nuclei-templates")
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

func PullTemplatesFromGithub(giturl string, proxy ...string) (string, error) {
	dir := consts.GetDefaultBaseHomeDir()
	nDir := filepath.Join(dir, "nuclei-templates")

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
		y := &yakit.YakScript{
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
		log.Infof("start to save yaml poc(nuclei) by name: %s", y.ScriptName)
		err = yakit.CreateOrUpdateYakScriptByName(db, y.ScriptName, y)
		if err != nil {
			total--
			log.Errorf("save nuclei yak script failed: %s", err)
		}
	}
	log.Infof("success for saving nuclei poc to database total: %v", total)
	return nil
}

func RemovePoCDatabase() error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("cannot fetch database: %s", db.Error)
	}

	if db := db.Model(&yakit.YakScript{}).Where(
		"(type = ?) AND (from_local = ?)",
		"nuclei", true).Unscoped().Delete(&yakit.YakScript{}); db.Error != nil {
		return db.Error
	}

	return nil
}

var NucleiOperationsExports = map[string]interface{}{
	"PullDatabase":   PullTemplatesFromGithub,
	"UpdateDatabase": LoadYamlPoCDatabase,
	"RemoveDatabase": RemovePoCDatabase,
	"AllPoC":         FetchCurrentNucleiTemplates,
	"PocVulToRisk":   PocVulToRisk,
	"GetPoCDir":      consts.GetNucleiTemplatesDir,
}
