package yakit

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"runtime"
	"time"
	"yaklang.io/yaklang/common/utils/lowhttp"

	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/ziputil"
	"yaklang.io/yaklang/common/yakgrpc/ypb"

	git "github.com/go-git/go-git/v5"
	gitClient "github.com/go-git/go-git/v5/plumbing/transport/client"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

type StoreMeta struct {
	Types []ModuleType `json:"types"`
}

type ModuleType struct {
	Name      string `json:"name"`
	VerboseCN string `json:"verbose_cn"`
	VerboseEn string `json:"verbose_en"`
	External  bool   `json:"external"`
	Dir       string `json:"dir"`
}

//var (
//	AvailableModuleTypes = []string{
//		"nuclei",
//		"yak",
//		"mitm",
//		"nmap",
//		"codec",
//		"crawler",
//		"packet-hack",
//		"brute",
//	}
//)

func GetDefaultScriptFileNameByType(t string) (string, error) {
	switch strings.ToLower(t) {
	case "yak":
		return "yak_module.yak", nil
	case "nuclei":
		return "nuclei.yaml", nil
	case "codec":
		return "codec.yak", nil
	case "port-scan":
		return "handle.yak", nil
	case "mitm":
		return "yak_mitm.yak", nil
	default:
		return "handle.yak", nil
	}
}

type YakModuleMeta struct {
	ModuleName           string           `json:"name" yaml:"name"`
	Tags                 []string         `json:"tags" yaml:"tags"`
	Help                 string           `json:"help" yaml:"help"`
	Author               string           `json:"author" yaml:"author"`
	ModuleFile           string           `json:"module_file" yaml:"module_file"`
	Params               []YakModuleParam `json:"params" yaml:"params"`
	Document             string           `json:"document" yaml:"document"`
	GeneralModuleVerbose string           `json:"general_module_verbose" yaml:"general_module_verbose"`
	GeneralModuleKey     string           `json:"general_module_key" yaml:"general_module_key"`
	IsGeneralModule      bool             `json:"is_general_module" yaml:"is_general_module"`
	EnablePluginSelector bool             `json:"enable_plugin_selector" yaml:"enable_plugin_selector"`
	PluginSelectorTypes  string           `json:"plugin_selector_types" yaml:"plugin_selector_types"`
}

type YakModuleParam struct {
	Name         string `json:"name" yaml:"name"`
	Verbose      string `json:"verbose" yaml:"verbose"`
	Description  string `json:"description" yaml:"description"`
	Type         string `json:"type" yaml:"type"`
	DefaultValue string `json:"default_value" yaml:"default_value"`
	Required     bool   `json:"required" yaml:"required"`
	Group        string `json:"group" yaml:"group"`
	ExtraSetting string `json:"extra_setting" yaml:"extra_setting"`
}

const (
	aliyunOSSBase = "https://yaklang.oss-cn-beijing.aliyuncs.com/yak/"
)

func UpdateYakitStore(db *gorm.DB, baseUrl string) error {
	var downloadUrl = baseUrl
	if downloadUrl == "" {
		ins, _ := url.Parse(aliyunOSSBase)
		if ins != nil {
			ins.Path = filepath.Join(ins.Path, "yakit-resources.zip")
			downloadUrl = ins.String()
		} else {
			downloadUrl = "https://yaklang.oss-cn-beijing.aliyuncs.com/yak/yakit-resources.zip"
		}
	}

	rsp, err := http.Get(downloadUrl)
	if err != nil {
		return err
	}

	localZip := filepath.Join(consts.GetDefaultYakitBaseDir(), "yakit-resources.zip")
	_ = os.RemoveAll(localZip)
	f, err := os.OpenFile(localZip, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return utils.Errorf("[%v] create failed: %s", localZip, err)
	}

	if rsp.Body != nil {
		io.Copy(f, rsp.Body)
		f.Close()
	} else {
		f.Close()
		return utils.Errorf("empty body for %s", downloadUrl)
	}

	localDir := filepath.Join(consts.GetDefaultYakitBaseDir(), "yakit-resources")
	// 解压 zip
	_ = os.RemoveAll(localDir)
	err = ziputil.DeCompress(localZip, localDir)
	if err != nil {
		return utils.Errorf("unzip %v failed: %s", localZip, err)
	}

	scripts, markdowns, err := LoadYakitResources(localDir)
	if err != nil {
		return utils.Errorf("load yakit resource failed: %s", err)
	}

	for _, r := range scripts {
		if db == nil {
			continue
		}
		err := CreateOrUpdateYakScriptByName(db, r.ScriptName, r)
		if err != nil {
			log.Errorf("save [%v] failed: %v", r.ScriptName, r)
		}
	}

	for _, m := range markdowns {
		if db == nil {
			continue
		}
		err := CreateOrUpdateMarkdownDoc(db, 0, m.YakScriptName, m)
		if err != nil {
			log.Errorf("save markdown[%v] failed: %v", m.YakScriptName, err)
		}
	}

	return nil
}

func LoadYakitResources(dirName string) ([]*YakScript, []*MarkdownDoc, error) {
	typesSchema := filepath.Join(dirName, "sources.json")
	raw, err := ioutil.ReadFile(typesSchema)
	if err != nil {
		return nil, nil, err
	}

	var sources StoreMeta
	err = json.Unmarshal(raw, &sources)
	if err != nil {
		return nil, nil, err
	}

	var scripts []*YakScript
	var mds []*MarkdownDoc
	for _, r := range sources.Types {
		log.Infof("start to load: %v [%v/%v]", r.Name, r.VerboseCN, r.VerboseEn)
		if r.External {
			log.Infof("external [%v] skipped", r.Name)
			continue
		}

		modDir := filepath.Join(dirName, r.Dir)
		fs, err := utils.ReadDirsRecursively(modDir)
		if err != nil {
			log.Infof("load yakit resource[%s] failed: %s", modDir, err)
			continue
		}

		// 设置模块加载白名单
		//if !utils.StringSliceContain(AvailableModuleTypes, r.Name) {
		//	log.Infof("skipped type[%v]", r.Name)
		//	continue
		//}

		for _, f := range fs {
			if !f.IsDir {
				log.Infof("skipped: %s", f.Path)
				continue
			}

			basePath := f.Path
			script, markdown, err := LoadPackage(r.Name, basePath)
			if err != nil {
				log.Warnf("load package failed for %s", err)
				continue
			}
			scripts = append(scripts, script)
			if markdown != nil {
				mds = append(mds, markdown)
			}
		}
	}
	return scripts, mds, nil
}

func LoadPackage(typeStr string, basePath string) (*YakScript, *MarkdownDoc, error) {
	// 检查每一个包
	// 处理源信息
	metaFile := utils.GetFirstExistedFile(
		filepath.Join(basePath, "meta.yaml"),
		filepath.Join(basePath, "meta.yml"),
		filepath.Join(basePath, "meta.json"),
	)
	if metaFile == "" {
		return nil, nil, errors.New("empty meta.yaml")
	}
	raw, err := ioutil.ReadFile(metaFile)
	if err != nil {
		return nil, nil, utils.Errorf("read %s failed: %s", metaFile, err)
	}
	var modMeta YakModuleMeta
	if strings.HasSuffix(metaFile, ".json") {
		err = json.Unmarshal(raw, &modMeta)
	} else {
		err = yaml.Unmarshal(raw, &modMeta)
	}
	if err != nil {
		return nil, nil, utils.Errorf("unmarshal module meta failed: %s", err)
	}
	if modMeta.Author == "" {
		modMeta.Author = "anonymous"
	}

	_, moduleName := path.Split(basePath)
	var script = &YakScript{
		ScriptName:           modMeta.ModuleName,
		Type:                 typeStr,
		Help:                 modMeta.Help,
		Author:               modMeta.Author,
		Tags:                 strings.Join(modMeta.Tags, ","),
		FromStore:            true,
		IsGeneralModule:      modMeta.IsGeneralModule,
		GeneralModuleVerbose: modMeta.GeneralModuleVerbose,
		GeneralModuleKey:     modMeta.GeneralModuleKey,
		EnablePluginSelector: modMeta.EnablePluginSelector,
		PluginSelectorTypes:  modMeta.PluginSelectorTypes,
	}
	if script.ScriptName == "" {
		script.ScriptName = fmt.Sprintf("%v[%v] @%v", moduleName, typeStr, modMeta.Author)
	}
	fileName := filepath.Join(basePath, modMeta.ModuleFile)
	raw, _ = ioutil.ReadFile(fileName)
	if raw == nil {
		return nil, nil, utils.Errorf("read modfile[%v] failed: %s", fileName, err)
	}

	var content string
	if !utf8.Valid(raw) {
		content = utils.EscapeInvalidUTF8Byte(raw)
	} else {
		content = string(raw)
	}
	script.Content = content

	var params []*ypb.YakScriptParam
	for _, r := range modMeta.Params {
		params = append(params, &ypb.YakScriptParam{
			Field:        r.Name,
			DefaultValue: r.DefaultValue,
			TypeVerbose:  r.Type,
			FieldVerbose: r.Verbose,
			Help:         r.Description,
			Required:     r.Required,
			Group:        r.Group,
			ExtraSetting: r.ExtraSetting,
		})
	}

	paramRaw, err := json.Marshal(params)
	if err != nil {
		return nil, nil, utils.Errorf("marshal failed: %s", err)
	}

	script.Params = strconv.Quote(string(paramRaw))

	if modMeta.Document != "" {
		docPath := filepath.Join(basePath, modMeta.Document)
		mdRaw, err := ioutil.ReadFile(docPath)
		if err != nil {
			return nil, nil, utils.Errorf("read doc[%s] failed: %s", docPath, err.Error())
		}
		mk := &MarkdownDoc{
			YakScriptId:   0,
			YakScriptName: script.ScriptName,
			Markdown:      utils.EscapeInvalidUTF8Byte([]byte(mdRaw)),
		}
		return script, mk, nil
	}

	return script, nil, nil
}

func LoadYakitFromLocalDir(f string) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("load yakit database failed")
	}
	scripts, mds, err := LoadYakitResources(f)
	if err != nil {
		return utils.Errorf("load yakit resource failed: %s", err)
	}

	for _, r := range scripts {
		if db == nil {
			continue
		}
		r.FromGit = fmt.Sprintf("git://%v", f)
		err := CreateOrUpdateYakScriptByName(db, r.ScriptName, r)
		if err != nil {
			log.Errorf("save [%v] failed: %v", r.ScriptName, r)
		}
	}

	for _, m := range mds {
		if db == nil {
			continue
		}
		err := CreateOrUpdateMarkdownDoc(db, 0, m.YakScriptName, m)
		if err != nil {
			log.Errorf("save markdown[%v] failed: %v", m.YakScriptName, err)
		}
	}

	return nil
}

func LoadYakitThirdpartySourceScripts(
	ctx context.Context, ghUrl string,
	proxy ...string,
) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Error("cannot fetch database ... ")
	}

	defaultBaseDir := consts.GetDefaultYakitBaseDir()
	u, err := url.Parse(ghUrl)
	if err != nil {
		return utils.Errorf("parse git-url failed: %s", err)
	}
	log.Infof("start to load url: %v", ghUrl)

	targetPath := u.EscapedPath()
	for {
		if strings.Contains(targetPath, "http://") {
			targetPath = strings.ReplaceAll(targetPath, "http://", "")
			continue
		}

		if strings.Contains(targetPath, "https://") {
			targetPath = strings.ReplaceAll(targetPath, "https://", "")
			continue
		}

		if strings.Contains(targetPath, "://") {
			targetPath = strings.ReplaceAll(targetPath, "://", "_")
			continue
		}

		if strings.Contains(targetPath, "../") {
			targetPath = strings.ReplaceAll(targetPath, "../", "_")
			continue
		}

		if strings.Contains(targetPath, "./") {
			targetPath = strings.ReplaceAll(targetPath, "./", "_")
			continue
		}

		break
	}
	switch runtime.GOOS {
	case "windows":
		targetPath = strings.ReplaceAll(targetPath, "/", "\\")
	}

	f := filepath.Join(defaultBaseDir, "repos", targetPath)
	log.Infof("prepare from local: %v", f)
	_ = os.RemoveAll(f)

	if len(proxy) > 0 {
		log.Infof("proxy: %v", proxy)
	}
	// 设置 client
	//client := utils.NewDefaultHTTPClient()
	// Create a custom http(s) client with your config
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		Proxy:           http.ProxyFromEnvironment,
	}
	if len(proxy) > 0 {
		u, err := url.Parse(proxy[0])
		if err != nil {
			return utils.Errorf("parse proxy[%v] failed: %s", proxy[0], err)
		}

		if !utils.MatchAnyOfSubString(u.Scheme, "http", "https", "socks5") {
			return utils.Errorf("proxy's schema invalid: %v", u.Scheme)
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

	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	gitClient.InstallProtocol("https", gitHttp.NewClient(client))
	gitClient.InstallProtocol("http", gitHttp.NewClient(client))

	r, err := git.PlainCloneContext(ctx, f, false, &git.CloneOptions{
		URL:      ghUrl,
		Depth:    1,
		Progress: os.Stdout,
	})
	if err != nil {
		return utils.Errorf("clone %v failed: %s", ghUrl, err)
	}
	_ = r

	scripts, mds, err := LoadYakitResources(f)
	if err != nil {
		return utils.Errorf("load yakit resource failed: %s", err)
	}

	for _, r := range scripts {
		if db == nil {
			continue
		}
		r.FromGit = ghUrl
		err := CreateOrUpdateYakScriptByName(db, r.ScriptName, r)
		if err != nil {
			log.Errorf("save [%v] failed: %v", r.ScriptName, r)
		}
	}

	for _, m := range mds {
		if db == nil {
			continue
		}
		err := CreateOrUpdateMarkdownDoc(db, 0, m.YakScriptName, m)
		if err != nil {
			log.Errorf("save markdown[%v] failed: %v", m.YakScriptName, err)
		}
	}

	return nil
}
