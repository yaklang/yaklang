package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/copier"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var nucleiLoaderCD = utils.NewCoolDown(10 * time.Second)

var buildinNucleiYakScriptParam = tools.BuildinNucleiYakScriptParam

func (s *Server) LoadNucleiTemplates(ctx context.Context, req *ypb.Empty) (*ypb.Empty, error) {
	nucleiLoaderCD.Do(func() {
		descs, err := tools.FetchCurrentNucleiTemplates()
		if err != nil {
			log.Errorf("fetch current nuclei template: %s", err)
			return
		}

		if len(descs) > 0 {
			consts.GetGormProfileDatabase().Model(&yakit.YakScript{}).Where("(type = ?) AND (from_local = ?) AND (is_external = true)", "nuclei", true).Unscoped().Delete(&yakit.YakScript{})
		}

		for _, r := range descs {
			raw, _ := json.Marshal(buildinNucleiYakScriptParam)
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
			err = yakit.CreateOrUpdateYakScriptByName(s.GetProfileDatabase(), y.ScriptName, y)
			if err != nil {
				log.Errorf("save nuclei yak script failed: %s", err)
			}
		}
	})
	return &ypb.Empty{}, nil
}

func (s *Server) AutoUpdateYakModule(_ *ypb.Empty, stream ypb.Yak_AutoUpdateYakModuleServer) error {
	err := s.Exec(&ypb.ExecRequest{
		Script: `yakit.AutoInitYakit()

yakit.Info("开始更新 yakit-store: %v", "github.com/yaklang/yakit-store")
err = yakit.UpdateYakitStore()
if err != nil {
    yakit.Error("更新 yakit-store 官方商店失败: %v", err)
}
yakit.Info("更新 yakit-store 成功")

yakit.Info("开始从 nuclei templates 中更新 yaml poc")
err = nuclei.UpdatePoC()
if err != nil {
	yakit.Error("update poc from github src failed: %s", err)
	yakit.Info("try clone https://github.com/projectdiscovery/nuclei-templates && ")
}

`,
	}, stream)
	if err != nil {
		return utils.Errorf("auto update nuclei poc failed: %s", err.Error())
	}

	_, err = s.LoadNucleiTemplates(stream.Context(), &ypb.Empty{})
	if err != nil {
		return utils.Errorf("auto-save nuclei pocs to db failed: %s", err.Error())
	}

	_, err = s.UpdateFromYakitResource(stream.Context(), &ypb.UpdateFromYakitResourceRequest{})
	if err != nil {
		log.Errorf("update yakit-resource failed: %s", err)
	}
	return nil
}

func (s *Server) GetYakScriptById(ctx context.Context, req *ypb.GetYakScriptByIdRequest) (*ypb.YakScript, error) {
	ins, err := yakit.GetYakScript(s.GetProfileDatabase(), req.GetId())
	if err != nil {
		return nil, err
	}
	return ins.ToGRPCModel(), nil
}

func (s *Server) GetYakScriptByName(ctx context.Context, req *ypb.GetYakScriptByNameRequest) (*ypb.YakScript, error) {
	ins, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), req.GetName())
	if err != nil {
		return nil, err
	}
	return ins.ToGRPCModel(), nil
}

func (s *Server) GetYakScriptByOnlineID(ctx context.Context, req *ypb.GetYakScriptByOnlineIDRequest) (*ypb.YakScript, error) {
	ins, err := yakit.GetYakScriptByUUID(s.GetProfileDatabase(), req.GetUUID())
	if err != nil {
		return nil, utils.Errorf("uuid or online_id all empty: %v(%v)", req.GetOnlineID(), req.GetUUID())
	}
	return ins.ToGRPCModel(), nil
}

func (s *Server) GetAvailableYakScriptTags(ctx context.Context, req *ypb.Empty) (*ypb.Fields, error) {
	stats, err := yaklib.NewTagStat()
	if err != nil {
		return nil, err
	}
	var fields ypb.Fields
	for _, v := range stats.All() {
		fields.Values = append(fields.Values, &ypb.FieldName{
			Name:    v.Name,
			Verbose: v.Name,
			Total:   int32(v.Count),
		})
	}
	return &fields, nil
}

func (s *Server) ForceUpdateAvailableYakScriptTags(ctx context.Context, req *ypb.Empty) (*ypb.Empty, error) {
	stats, err := yaklib.NewTagStat()
	if err != nil {
		return nil, err
	}
	err = stats.ForceUpdate()
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QueryYakScriptByYakScriptName(req *ypb.QueryYakScriptRequest, stream ypb.Yak_QueryYakScriptByYakScriptNameServer) error {
	var names [][]string
	if len(req.GetIncludedScriptNames()) > 0 {
		names = utils.SliceGroup(req.GetIncludedScriptNames(), 50)
	}

	for _, group := range names {
		var newRequest ypb.QueryYakScriptRequest
		err := copier.Copy(&newRequest, req)
		if err != nil {
			return err
		}
		r := &newRequest
		r.IncludedScriptNames = group
		rsp, err := s.QueryYakScript(stream.Context(), r)
		if err != nil {
			log.Error(err)
		}
		if rsp != nil {
			for _, data := range rsp.Data {
				stream.Send(data)
			}
		}
	}
	return nil
}

func (s *Server) QueryYakScript(ctx context.Context, req *ypb.QueryYakScriptRequest) (*ypb.QueryYakScriptResponse, error) {
	if req.GetNoResultReturn() {
		return &ypb.QueryYakScriptResponse{
			Pagination: req.GetPagination(),
			Total:      0,
			Data:       nil,
		}, nil
	}
	p, data, err := yakit.QueryYakScript(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}

	rsp := &ypb.QueryYakScriptResponse{
		Pagination: &ypb.Paging{
			Page:    int64(p.Page),
			Limit:   int64(p.Limit),
			OrderBy: req.Pagination.OrderBy,
			Order:   req.Pagination.Order,
		},
		Total: int64(p.TotalRecord),
	}

	for _, d := range data {
		rsp.Data = append(rsp.Data, d.ToGRPCModel())
	}
	return rsp, nil
}

func GRPCYakScriptToYakitScript(script *ypb.YakScript) *yakit.YakScript {
	raw, _ := json.Marshal(script.Params)
	if script.IsGeneralModule && script.GeneralModuleKey == "" {
		script.GeneralModuleKey = script.ScriptName
		script.GeneralModuleVerbose = script.ScriptName
	}
	return &yakit.YakScript{
		ScriptName:           script.ScriptName,
		Type:                 script.Type,
		Content:              script.Content,
		Level:                script.Level,
		Params:               strconv.Quote(string(raw)),
		Help:                 script.Help,
		Author:               script.Author,
		Tags:                 script.Tags,
		IsHistory:            script.IsHistory,
		IsGeneralModule:      script.IsGeneralModule,
		GeneralModuleKey:     script.GeneralModuleKey,
		GeneralModuleVerbose: script.GeneralModuleVerbose,
		EnablePluginSelector: script.EnablePluginSelector,
		PluginSelectorTypes:  script.PluginSelectorTypes,
	}
}

func (s *Server) SaveYakScript(ctx context.Context, script *ypb.YakScript) (*ypb.YakScript, error) {
	if script.Type == "nuclei" {
		script.Params = buildinNucleiYakScriptParam
	}

	switch script.Type {
	case "yak", "mitm", "port-scan":
		_, err := antlr4yak.New().FormattedAndSyntaxChecking(script.GetContent())
		if err != nil {
			return nil, utils.Errorf("save plugin failed! content is invalid(潜在语法错误): %s", err)
		}
	}

	err := yakit.CreateOrUpdateYakScriptByName(s.GetProfileDatabase(), script.ScriptName, GRPCYakScriptToYakitScript(script))
	if err != nil {
		return nil, utils.Errorf("create or update yakscript failed: %s", err.Error())
	}

	_ = yakit.CreateOrUpdateYakScriptByName(s.GetProfileDatabase(), script.ScriptName, map[string]interface{}{
		"enable_plugin_selector": script.EnablePluginSelector,
		"plugin_selector_types":  script.PluginSelectorTypes,
	})

	//if !script.IsGeneralModule {
	//	err = yakit.CreateOrUpdateYakScriptByName(s.GetProfileDatabase(),script.ScriptName, map[string]interface{}{
	//		"is_general_module": script.IsGeneralModule,
	//	})
	//	if err != nil {
	//		log.Errorf("update is_general_module failed: %s", err)
	//	}
	//}

	res, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), script.ScriptName)
	if err != nil {
		return nil, utils.Errorf("query saved yak script failed: %s", err)
	}

	return res.ToGRPCModel(), nil
}

func (s *Server) IgnoreYakScript(ctx context.Context, req *ypb.DeleteYakScriptRequest) (*ypb.Empty, error) {
	err := yakit.IgnoreYakScriptByID(s.GetProfileDatabase(), req.GetId(), true)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) UnIgnoreYakScript(ctx context.Context, req *ypb.DeleteYakScriptRequest) (*ypb.Empty, error) {
	err := yakit.IgnoreYakScriptByID(s.GetProfileDatabase(), req.GetId(), false)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeleteYakScript(ctx context.Context, req *ypb.DeleteYakScriptRequest) (*ypb.Empty, error) {
	for _, i := range req.GetIds() {
		_ = yakit.DeleteYakScriptByID(s.GetProfileDatabase(), i)
	}
	err := yakit.DeleteYakScriptByID(s.GetProfileDatabase(), req.Id)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func ConvertYakScriptToExecRequest(req *ypb.ExecRequest, script *yakit.YakScript, batchMode bool) (*ypb.ExecRequest, []func(), error) {
	var defers []func()
	switch script.Type {
	case "mitm", "port-scan":
		params := req.Params
		params = append(params, &ypb.ExecParamItem{Key: "--plugin", Value: script.ScriptName})
		return &ypb.ExecRequest{
			Params: params,
			Script: generalBatchExecutor,
		}, defers, nil
	case "yak":
		return &ypb.ExecRequest{
			Params:   req.Params,
			Script:   script.Content,
			ScriptId: script.ScriptName,
		}, defers, nil
	case "nuclei":
		// 批量模式不太一样
		params := req.Params
		if batchMode {
			params = append(params, &ypb.ExecParamItem{Key: "--plugin", Value: script.ScriptName})
		} else {
			pocName := script.ScriptName //= script.LocalPath
			params = append(params, &ypb.ExecParamItem{
				Key:   "pocName",
				Value: pocName,
			})

			_, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(script.Content)
			if err != nil {
				return nil, nil, utils.Errorf("pocFile: %v is not valid nuclei yaml poc", script.ScriptName)
			}
			//var rawTemp templates.Template
			//err := yaml.Unmarshal([]byte(script.Content), &rawTemp)
			//if len(rawTemp.Workflow.Workflows) > 0 || len(rawTemp.Workflows) > 0 || rawTemp.CompiledWorkflow != nil {
			//	if batchMode {
			//		return nil, nil, utils.Errorf("batch mode support workflow nuclei")
			//	}
			//	params = append(params, &ypb.ExecParamItem{
			//		Key: "isWorkflow",
			//	})
			//}
		}

		if batchMode {
			newReq := &ypb.ExecRequest{
				Params:   params,
				Script:   generalBatchExecutor,
				ScriptId: script.ScriptName,
			}
			return newReq, defers, nil
		}

		newReq := &ypb.ExecRequest{
			Params:   params,
			Script:   nucleiExecutor,
			ScriptId: script.ScriptName,
		}
		return newReq, defers, nil
	default:
		return nil, defers, utils.Errorf("cannot exec yak script type[%v]", script.Type)
	}
}

func (s *Server) ExecYakScript(req *ypb.ExecRequest, stream ypb.Yak_ExecYakScriptServer) error {
	var (
		script *yakit.YakScript
		err    error
	)
	if req.GetYakScriptId() > 0 {
		script, err = yakit.GetYakScript(s.GetProfileDatabase(), req.GetYakScriptId())
		if err != nil {
			return utils.Errorf("cannot fetch yak script(ExecYakScript): %s", err.Error())
		}
	} else if req.GetScriptId() != "" {
		script, err = yakit.GetYakScriptByName(s.GetProfileDatabase(), req.GetScriptId())
		if err != nil {
			return utils.Errorf("cannot fetch yak script by name (ExecYakScript) failed: %s, (%v)", err, req.GetScriptId())
		}
	}

	if script == nil {
		return utils.Errorf("cannot fetch yak script (ExecYakScript) failed: %s", spew.Sdump(req))
	}

	switch strings.ToLower(script.Type) {
	case "packet-hack":
		log.Infof("execute script[packet-pack]...: %v", script.ScriptName)
		var request string
		var response string
		var isHttps bool
		funk.ForEach(req.Params, func(i *ypb.ExecParamItem) {
			switch i.Key {
			case "request":
				request = i.Value
			case "response":
				response = i.Value
			case "isHttps":
				isHttps, _ = strconv.ParseBool(i.Value)
			}
		})
		params, code, err := s.generatePacketHackParams(&ypb.ExecutePacketYakScriptParams{
			ScriptName: script.ScriptName,
			IsHttps:    isHttps,
			Request:    []byte(request),
			Response:   []byte(response),
		})
		if err != nil {
			return err
		}
		return s.Exec(&ypb.ExecRequest{
			Script: code,
			Params: params,
		}, stream)
	case "codec":
		return execTestCaseMITMHooksCaller(
			stream.Context(),
			script, req.Params, s.GetProfileDatabase(),
			func(r *ypb.ExecResult) error {
				return stream.Send(r)
			},
		)
	case "mitm", "nuclei":
		// yak / nuclei
		log.Infof("start to exec yak script... : %v", script.ScriptName)
		var target string
		for _, paramItem := range req.GetParams() {
			if paramItem.Key == "target" {
				target = paramItem.Value
			}
		}
		return s.execScriptWithRequest(script.ScriptName, target, stream, nil)
	case "port-scan":
		params, code, err := s.generatePortScanParams(script.ScriptName, req.GetParams())
		if err != nil {
			return err
		}
		req.Params = params
		req.Script = code
		req.YakScriptId = int64(script.ID)
		return s.ExecWithContext(stream.Context(), req, stream)
	default:
		req.ScriptId = script.ScriptName
		req.YakScriptId = int64(script.ID)
		return s.ExecWithContext(stream.Context(), req, stream)
	}
}

const defaultMarkdownDocument = `# [%v]%v 's Document

Author: %v

'`

func typeToDirname(s string) string {
	switch utils.ToLowerAndStrip(s) {
	case "nuclei":
		return "yak_nuclei"
	case "mitm":
		return "yak_mitm"
	case "yak":
		return "yak_module"
	case "codec":
		return "yak_codec"
	case "packet-hack":
		return "yak_packet"
	case "port-scan":
		return "yak_portscan"
	}
	return "default"
}

func ReplaceString(s string) string {
	if strings.Contains(s, "|") {
		s = strings.Replace(s, "|", "", 1)
	}
	if strings.Contains(s, "\\") {
		s = strings.Replace(s, "\\", "", 1)
	}
	if strings.Contains(s, "/") {
		s = strings.Replace(s, "/", "", 1)
	}
	if strings.Contains(s, ":") {
		s = strings.Replace(s, ":", "", 1)
	}
	if strings.Contains(s, "*") {
		s = strings.Replace(s, "*", "", 1)
	}
	if strings.Contains(s, "?") {
		s = strings.Replace(s, "?", "", 1)
	}
	if strings.Contains(s, "\"") {
		s = strings.Replace(s, "\"", "", 1)
	}
	if strings.Contains(s, "<") {
		s = strings.Replace(s, "<", "", 1)
	}
	if strings.Contains(s, ">") {
		s = strings.Replace(s, ">", "", 1)
	}
	return s
}

func (s *Server) ExportYakScript(ctx context.Context, req *ypb.ExportYakScriptRequest) (*ypb.ExportYakScriptResponse, error) {
	if !req.GetAll() && req.GetYakScriptIds() == nil && req.GetYakScriptId() == 0 {
		return nil, utils.Errorf("params empty")
	}
	scripts, err := yakit.GetYakScriptList(s.GetProfileDatabase(), req.GetYakScriptId(), req.YakScriptIds)
	if err != nil {
		return nil, err
	}
	dir := req.GetOutputDir()

	for _, v := range scripts {
		outputPluginDir := v.ScriptName
		if req.GetYakScriptId() > 0 {
			outputPluginDir = req.GetOutputPluginDir()
		}
		dirRet, err := s.ExportYakPluginBatch(v, req.GetOutputDir(), ReplaceString(outputPluginDir))
		if req.GetYakScriptId() > 0 {
			dir = dirRet
		}
		if err != nil {
			return nil, utils.Errorf(v.ScriptName + err.Error())
		}
	}

	return &ypb.ExportYakScriptResponse{OutputDir: dir}, nil
}

func (s *Server) ExportYakPluginBatch(script *yakit.YakScript, dir, OutputPluginDir string) (string, error) {
	if dir == "" {
		dir = filepath.Join(consts.GetDefaultYakitBaseDir(), "user-plugins", script.Type)
		os.MkdirAll(dir, os.ModePerm)
	} else {
		if OutputPluginDir == "" {
			return "", utils.Error("output plugin dir is not set")
		}
		dir = filepath.Join(dir, typeToDirname(script.Type), OutputPluginDir)
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return "", utils.Errorf("create dir[%v] failed: %s", dir, err)
		}
	}
	modFile, err := yakit.GetDefaultScriptFileNameByType(script.Type)
	if err != nil {
		return "", err
	}

	var (
		modPath  = filepath.Join(dir, modFile)
		docFile  = "document.md"
		metaFile = "meta.json"
		docPath  = filepath.Join(dir, docFile)
		metaPath = filepath.Join(dir, metaFile)
	)

	scriptName := script.OnlineScriptName
	if scriptName == "" {
		scriptName = script.ScriptName
	}

	_ = script
	meta := yakit.YakModuleMeta{
		ModuleName:           scriptName,
		Tags:                 strings.Split(script.Tags, ","),
		Help:                 script.Help,
		Author:               script.Author,
		ModuleFile:           modFile,
		Document:             docFile,
		GeneralModuleVerbose: script.GeneralModuleVerbose,
		GeneralModuleKey:     script.GeneralModuleKey,
		IsGeneralModule:      script.IsGeneralModule,
	}
	if script.OnlineScriptName != "" {
		meta.ModuleName = script.OnlineScriptName
	}

	var execParams []*ypb.YakScriptParam
	if script.Params != "" {
		paramJson, _ := strconv.Unquote(script.Params)
		if paramJson == "" {
			paramJson = script.Params
		}
		err = json.Unmarshal([]byte(paramJson), &execParams)
		if err != nil {
			return "", utils.Errorf("unmarshal script params failed: %s", err)
		}
	}

	for _, p := range execParams {
		meta.Params = append(meta.Params, yakit.YakModuleParam{
			Name:         p.Field,
			Verbose:      p.FieldVerbose,
			Description:  p.Help,
			Type:         p.TypeVerbose,
			DefaultValue: p.DefaultValue,
			Required:     p.Required,
			Group:        p.Group,
			ExtraSetting: p.ExtraSetting,
		})
	}

	// 保存 meta
	metaRaw, err := json.MarshalIndent(meta, "", "    ")
	if err != nil {
		return "", utils.Errorf("marshal meta.json failed: %s", err)
	}
	os.RemoveAll(metaPath)
	err = ioutil.WriteFile(metaPath, metaRaw, 0o666)
	if err != nil {
		return "", utils.Errorf("write meta.json failed: %s", err)
	}

	// 保存文档
	documentRaw := []byte(fmt.Sprintf(
		defaultMarkdownDocument,
		script.Type, scriptName, script.Author,
	))
	markdownFile, _ := yakit.GetMarkdownDocByName(s.GetProfileDatabase(), int64(script.ID), scriptName)
	if markdownFile != nil {
		documentRaw = []byte(markdownFile.Markdown)
	}

	os.RemoveAll(docPath)
	err = ioutil.WriteFile(docPath, documentRaw, 0o666)
	if err != nil {
		return "", utils.Errorf("write document failed: %s", err)
	}

	// 保存脚本内容
	scriptStr, _ := strconv.Unquote(script.Content)
	if scriptStr == "" {
		scriptStr = script.Content
	}
	os.RemoveAll(modPath)
	err = ioutil.WriteFile(modPath, []byte(scriptStr), 0o666)
	if err != nil {
		return "", utils.Errorf("write script failed: %s", err)
	}

	newScripts, _, err := yakit.LoadPackage(script.Type, dir)
	if err != nil {
		return "", utils.Errorf("verify output dir failed: %s", err.Error())
	}
	_ = newScripts
	return dir, nil
}

func (s *Server) QueryYakScriptExecResult(ctx context.Context, req *ypb.QueryYakScriptExecResultRequest) (*ypb.QueryYakScriptExecResultResponse, error) {
	p, res, err := yakit.QueryExecResult(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}

	var results []*ypb.ExecResult
	for _, r := range res {
		result := r.ToGRPCModel()
		if result == nil {
			continue
		}
		result.Hash = utils.CalcSha1(r.YakScriptName, r.Raw, r.ID)
		results = append(results, result)
	}

	return &ypb.QueryYakScriptExecResultResponse{
		Pagination: req.Pagination,
		Total:      int64(p.TotalRecord),
		Data:       results,
	}, nil
}

func (s *Server) QueryYakScriptNameInExecResult(ctx context.Context, req *ypb.Empty) (*ypb.YakScriptNames, error) {
	var res []*yakit.ExecResult
	s.GetProjectDatabase().Raw("select distinct yak_script_name from exec_results").Scan(&res)
	var plugins []string
	for _, r := range res {
		plugins = append(plugins, r.YakScriptName)
	}
	return &ypb.YakScriptNames{YakScriptNames: plugins}, nil
}

func (s *Server) DeleteYakScriptExecResult(ctx context.Context, req *ypb.DeleteYakScriptExecResultRequest) (*ypb.Empty, error) {
	for _, i := range req.GetId() {
		_ = yakit.DeleteExecResultByID(s.GetProjectDatabase(), i)
	}

	if req.GetYakScriptName() != "" {
		_ = yakit.DeleteExecResultByYakScriptName(s.GetProjectDatabase(), req.GetYakScriptName())
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetYakScriptTagsAndType(ctx context.Context, req *ypb.Empty) (*ypb.GetYakScriptTagsAndTypeResponse, error) {
	var tagsAndType ypb.GetYakScriptTagsAndTypeResponse
	onlineType, _ := yakit.YakScriptType(s.GetProfileDatabase())
	db := consts.GetGormProfileDatabase()
	onlineTags := s.QueryYakScriptTagsGroup(db)

	if onlineTags == nil && onlineType == nil {
		return nil, utils.Errorf("GetYakScriptTagsAndTypeResponse Empty")
	}
	for _, v := range onlineType {
		tagsAndType.Type = append(tagsAndType.Type, &ypb.TagsAndType{
			Value: v.Value,
			Total: int32(v.Count),
		})
	}

	for _, v := range onlineTags {
		if v.Total > 1 {
			tagsAndType.Tag = append(tagsAndType.Tag, &ypb.TagsAndType{
				Value: v.Value,
				Total: int32(v.Total),
			})
		}
	}

	return &tagsAndType, nil
}

func (s *Server) DeleteYakScriptExec(ctx context.Context, req *ypb.Empty) (*ypb.Empty, error) {
	err := yakit.DeleteExecResult(s.GetProjectDatabase())
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetYakScriptTags(c context.Context, req *ypb.Empty) (*ypb.GetYakScriptTagsResponse, error) {
	var tags ypb.GetYakScriptTagsResponse
	/*onlineTags, err := yakit.YakScriptTags(s.GetProfileDatabase(), " and type in ( 'port-scan', 'mitm')", "")
	if onlineTags == nil {
		return nil, err
	}*/
	/*for _, v := range onlineTags {
		tags.Tag = append(tags.Tag, &ypb.Tags{
			Value: v.Value,
			Total: int32(v.Count),
		})
	}*/
	db := consts.GetGormProfileDatabase()
	db = db.Where("type in ( 'port-scan', 'mitm') ")
	tags.Tag = s.QueryYakScriptTagsGroup(db)
	if tags.Tag == nil {
		return nil, utils.Errorf("Response Empty")
	}

	return &tags, nil
}

func (s *Server) QueryYakScriptLocalAndUser(c context.Context, req *ypb.QueryYakScriptLocalAndUserRequest) (*ypb.QueryYakScriptLocalAndUserResponse, error) {
	rsp := &ypb.QueryYakScriptLocalAndUserResponse{}

	if req.GetOnlineBaseUrl() == "" || req.GetUserId() == 0 {
		return nil, utils.Errorf("params is empty")
	}
	db := consts.GetGormProfileDatabase()
	db = db.Where("online_base_url = ? and user_id = ? ", req.GetOnlineBaseUrl(), req.GetUserId())
	db = db.Or("online_id < ? ", 1)

	data := yakit.YieldYakScripts(db, context.Background())

	for d := range data {
		rsp.Data = append(rsp.Data, d.ToGRPCModel())
	}

	return rsp, nil
}

func (s *Server) QueryYakScriptByOnlineGroup(c context.Context, req *ypb.QueryYakScriptByOnlineGroupRequest) (*ypb.QueryYakScriptLocalAndUserResponse, error) {
	if req.GetOnlineGroup() == "" {
		return nil, utils.Errorf("params is empty")
	}
	rsp := &ypb.QueryYakScriptLocalAndUserResponse{}

	db := consts.GetGormProfileDatabase()
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"online_group",
	}, strings.Split(req.GetOnlineGroup(), ","), false)
	data := yakit.YieldYakScripts(db, context.Background())

	for d := range data {
		rsp.Data = append(rsp.Data, d.ToGRPCModel())
	}

	return rsp, nil
}

func (s *Server) QueryYakScriptLocalAll(c context.Context, req *ypb.Empty) (*ypb.QueryYakScriptLocalAndUserResponse, error) {
	rsp := &ypb.QueryYakScriptLocalAndUserResponse{}

	db := consts.GetGormProfileDatabase()
	data := yakit.YieldYakScripts(db, context.Background())
	for d := range data {
		rsp.Data = append(rsp.Data, d.ToGRPCModel())
	}
	return rsp, nil
}

func (s *Server) QueryYakScriptByNames(ctx context.Context, req *ypb.QueryYakScriptByNamesRequest) (*ypb.QueryYakScriptByNamesResponse, error) {
	ret := &ypb.QueryYakScriptByNamesResponse{}
	for _, y := range yakit.QueryYakScriptByNames(s.GetProfileDatabase(), req.YakScriptName...) {
		ret.Data = append(ret.Data, y.ToGRPCModel())
	}
	return ret, nil
}

func (s *Server) QueryYakScriptTagsGroup(db *gorm.DB) []*ypb.Tags {
	var tag []*ypb.Tags
	tagData := make(map[string]int64)
	db = db.Where("tags <> '' and tags <> '\"\"' and tags <> 'null' and is_history = '0' and ignored = '0'  ")
	yakScriptData := yakit.YieldYakScripts(db, context.Background())
	for v := range yakScriptData {
		var tags []string
		tags = strings.Split(v.Tags, ",")
		for _, tag := range utils.RemoveRepeatStringSlice(tags) {
			lowerTag := strings.ToLower(tag)
			tagData[lowerTag]++
		}
	}
	for k, v := range tagData {
		tagName := k
		tagCount := v
		tag = append(tag, &ypb.Tags{
			Value: tagName,
			Total: int32(tagCount),
		})
	}
	sort.SliceStable(tag, func(i, j int) bool {
		return tag[i].Total > tag[j].Total
	})
	return tag
}

func (s *Server) QueryYakScriptByIsCore(ctx context.Context, req *ypb.QueryYakScriptByIsCoreRequest) (*ypb.QueryYakScriptByIsCoreResponse, error) {
	ret := &ypb.QueryYakScriptByIsCoreResponse{}
	for _, y := range yakit.QueryYakScriptByIsCore(s.GetProfileDatabase(), req.IsCorePlugin) {
		ret.Data = append(ret.Data, y.ToGRPCModel())
	}
	return ret, nil
}

func (s *Server) QueryYakScriptRiskDetailByCWE(ctx context.Context, req *ypb.QueryYakScriptRiskDetailByCWERequest) (*ypb.QueryYakScriptRiskDetailByCWEResponse, error) {
	ret := &ypb.QueryYakScriptRiskDetailByCWEResponse{}
	if req.CWEId != "" {
		db := consts.GetGormCVEDatabase()
		if db == nil {
			log.Error("cannot found database (cve db)")
			return ret, nil
		}
		cwe, err := cveresources.GetCWE(db, req.CWEId)
		if err != nil {
			log.Errorf("get cwe failed: %s", err)
			return ret, nil
		}
		return &ypb.QueryYakScriptRiskDetailByCWEResponse{
			CWEId:       cwe.IdStr,
			RiskType:    cwe.NameZh,
			Description: cwe.Description,
			CWESolution: cwe.CWESolution,
		}, nil
	}
	return ret, nil
}

func (s *Server) YakScriptRiskTypeList(context.Context, *ypb.Empty) (*ypb.YakScriptRiskTypeListResponse, error) {
	ret := &ypb.YakScriptRiskTypeListResponse{}
	riskTypeList := map[string]string{
		"SQL注入":      "89",
		"XSS":        "79",
		"命令执行/注入":    "77",
		"代码执行/注入":    "94",
		"CSRF":       "352",
		"文件包含/读取/下载": "41",
		"文件写入/上传":    "434",
		"XML外部实体攻击":  "91",
		"反序列化":       "502",
		"未授权访问":      "552",
		"路径遍历":       "22",
		"敏感信息泄漏":     "200",
		"身份验证错误":     "305",
		"垂直/水平权限提升":  "271",
		"逻辑漏洞":       "840",
		"默认配置漏洞":     "1188",
		"弱口令":        "1391",
		"SSRF":       "918",
		"其他漏洞检测":     "",
		"合规检测":       "",
	}
	for k, v := range riskTypeList {
		ret.Data = append(ret.Data, &ypb.RiskTypeLists{
			RiskType: k,
			CWEId:    v,
		})
	}
	return ret, nil
}

func GRPCYakScriptToYakScript(script *ypb.SaveNewYakScriptRequest) map[string]interface{} {
	if script.IsGeneralModule && script.GeneralModuleKey == "" {
		script.GeneralModuleKey = script.ScriptName
		script.GeneralModuleVerbose = script.ScriptName
	}
	Data := map[string]interface{}{
		"script_name":            script.ScriptName,
		"type":                   script.Type,
		"content":                script.Content,
		"level":                  script.Level,
		"help":                   script.Help,
		"tags":                   script.Tags,
		"is_history":             script.IsHistory,
		"is_general_module":      script.IsGeneralModule,
		"general_module_key":     script.GeneralModuleKey,
		"general_module_verbose": script.GeneralModuleVerbose,
		"risk_type":              script.RiskType,
		"risk_annotation":        script.RiskAnnotation,
		"enable_plugin_selector": script.EnablePluginSelector,
		"plugin_selector_types":  script.PluginSelectorTypes,
	}
	if len(script.Params) > 0 {
		raw, _ := json.Marshal(script.Params)
		Data["params"] = strconv.Quote(string(raw))
	} else {
		Data["params"] = ""
	}
	if script.RiskDetail != nil {
		riskDetail, _ := json.Marshal(script.RiskDetail)
		Data["risk_detail"] = string(riskDetail)
	} else {
		Data["risk_detail"] = ""
	}
	return Data
}

func (s *Server) SaveNewYakScript(ctx context.Context, script *ypb.SaveNewYakScriptRequest) (*ypb.YakScript, error) {
	if script.Type == "nuclei" {
		script.Params = buildinNucleiYakScriptParam
	}

	switch script.Type {
	case "yak", "mitm", "port-scan":
		_, err := antlr4yak.New().FormattedAndSyntaxChecking(script.GetContent())
		if err != nil {
			return nil, utils.Errorf("save plugin failed! content is invalid(潜在语法错误): %s", err)
		}
	}
	script.ScriptName = strings.TrimSpace(script.ScriptName)

	yakScript, _ := yakit.GetYakScriptByName(s.GetProfileDatabase(), script.ScriptName)

	if script.Id > 0 {
		if yakScript != nil && int64(yakScript.ID) != script.Id {
			return nil, utils.Errorf("save plugin failed! 插件名重复")
		}
		yakScript, _ = yakit.GetYakScript(s.GetProfileDatabase(), script.Id)
		if yakScript == nil {
			return nil, utils.Errorf("更新插件不存在")
		}
		var err error
		if (script.ScriptName != yakScript.ScriptName && len(yakScript.OnlineBaseUrl) <= 0) || yakScript.ScriptName == script.ScriptName {
			// 更新
			err = yakit.CreateOrUpdateYakScript(s.GetProfileDatabase(), script.Id, GRPCYakScriptToYakScript(script))
		} else {
			err = yakit.CreateOrUpdateYakScriptByName(s.GetProfileDatabase(), script.ScriptName, GRPCYakScriptToYakScript(script))
		}
		if err != nil {
			return nil, utils.Errorf("update yakScript failed: %s", err.Error())
		}
	} else {
		if yakScript != nil {
			return nil, utils.Errorf("save plugin failed! 插件名重复")
		}
		err := yakit.CreateOrUpdateYakScriptByName(s.GetProfileDatabase(), script.ScriptName, GRPCYakScriptToYakScript(script))
		if err != nil {
			return nil, utils.Errorf("create or update yakScript failed: %s", err.Error())
		}
	}

	res, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), script.ScriptName)
	if err != nil {
		return nil, utils.Errorf("query saved yak script failed: %s", err)
	}

	return res.ToGRPCModel(), nil
}

func (s *Server) ExportLocalYakScript(ctx context.Context, req *ypb.ExportLocalYakScriptRequest) (*ypb.ExportLocalYakScriptResponse, error) {
	db := yakit.QueryExportYakScript(s.GetProfileDatabase(), req)
	scripts := yakit.YieldYakScripts(db, context.Background())
	dir := req.GetOutputDir()

	for v := range scripts {
		outputPluginDir := v.ScriptName
		dirRet, err := s.ExportYakPluginBatch(v, req.GetOutputDir(), ReplaceString(outputPluginDir))
		if len(req.GetYakScriptIds()) == 1 {
			dir = dirRet
		}
		if err != nil {
			log.Errorf("export [%s] failed: %s", v.ScriptName, err.Error())
		}
	}

	return &ypb.ExportLocalYakScriptResponse{OutputDir: dir}, nil
}

func (s *Server) ImportYakScript(req *ypb.ImportYakScriptRequest, stream ypb.Yak_ImportYakScriptServer) error {
	if len(req.Dirs) <= 0 {
		return utils.Errorf("params is empty")
	}
	var (
		progress                 float64
		count                    int
		message                  string
		errorCount, successCount int
	)
	messageType := "success"
	stream.Send(&ypb.ImportYakScriptResult{
		Progress:    0,
		Message:     "initializing",
		MessageType: "",
	})
	defer func() {
		if errorCount > 0 {
			message += fmt.Sprintf("执行失败: %v 个", errorCount)
			messageType = "finalError"
		}
		if successCount > 0 {
			message += fmt.Sprintf("执行成功: %v 个", successCount)
		}
		if message == "" {
			message = "finished"
		}

		stream.Send(&ypb.ImportYakScriptResult{
			Progress:    1,
			Message:     message,
			MessageType: messageType,
		})
	}()
	for _, dir := range req.Dirs {
		typeStr := yakit.YakScriptLocalType(dir)
		if typeStr == "" {
			stream.Send(&ypb.ImportYakScriptResult{
				Message:     fmt.Sprintf("import [%s] yakScript  failed: %s", dir, "文件名不符合上传"),
				MessageType: "error",
			})
			continue
		}
		total := yakit.YakScriptLocalTotal(req.Dirs)
		modDir := filepath.Join(dir)
		fs, err := utils.ReadDirsRecursively(modDir)
		if err != nil {
			stream.Send(&ypb.ImportYakScriptResult{
				Message:     fmt.Sprintf("import [%s] yakScript  failed: %s", dir, err),
				MessageType: "error",
			})
			continue
		}

		db := consts.GetGormProfileDatabase()
		if db == nil {
			return utils.Errorf("load yakit database failed")
		}

		for _, f := range fs {
			if total > 0 {
				progress = float64(count) / float64(total)
			}
			count++
			if count%100 == 0 {
				time.Sleep(1 * time.Second)
			}
			if !f.IsDir {
				errorCount++
				stream.Send(&ypb.ImportYakScriptResult{
					Progress:    progress,
					Message:     fmt.Sprintf("import [%s] yakScript failed ", dir),
					MessageType: "error",
				})
			}
			script, markdown, err := yakit.LoadPackage(typeStr, f.Path)
			if err != nil {
				errorCount++
				stream.Send(&ypb.ImportYakScriptResult{
					Progress:    progress,
					Message:     fmt.Sprintf("import [%s] yakScript  failed: %s", dir, err.Error()),
					MessageType: "error",
				})
			}
			if script != nil && script.Type != "" {
				err = yakit.CreateOrUpdateYakScriptByName(db, script.ScriptName, script)
				if err != nil {
					errorCount++
					stream.Send(&ypb.ImportYakScriptResult{
						Progress:    progress,
						Message:     fmt.Sprintf("import [%s] yakScript  failed: %s", dir, filepath.Base(f.Path)),
						MessageType: "error",
					})
				} else {
					successCount++
					stream.Send(&ypb.ImportYakScriptResult{
						Progress:    progress,
						Message:     fmt.Sprintf("import [%s] yakScript  success: %s", dir, filepath.Base(f.Path)),
						MessageType: "success",
					})
				}
			}
			if markdown != nil {
				err = yakit.CreateOrUpdateMarkdownDoc(db, 0, markdown.YakScriptName, markdown)
				if err != nil {
					errorCount++
					stream.Send(&ypb.ImportYakScriptResult{
						Progress:    progress,
						Message:     fmt.Sprintf("import [%s] yakScript  failed: %s", dir, filepath.Base(f.Path)),
						MessageType: "error",
					})
				} else {
					successCount++
					stream.Send(&ypb.ImportYakScriptResult{
						Progress:    progress,
						Message:     fmt.Sprintf("import [%s] yakScript  success: %s", dir, filepath.Base(f.Path)),
						MessageType: "success",
					})
				}
			}
		}

	}

	return nil
}

func (s *Server) ExportYakScriptLocal(req *ypb.ExportLocalYakScriptRequest, stream ypb.Yak_ExportYakScriptLocalServer) error {
	var (
		progress                 float64
		count                    int
		total                    int
		message                  string
		errorCount, successCount int
	)
	db := yakit.QueryExportYakScript(s.GetProfileDatabase(), req)
	scripts := yakit.YieldYakScripts(db, context.Background())
	db.Count(&total)

	dir := req.GetOutputDir()
	messageType := "success"
	stream.Send(&ypb.ExportYakScriptLocalResponse{
		Progress:    0,
		Message:     "initializing",
		MessageType: "",
	})
	defer func() {
		if errorCount > 0 {
			message += fmt.Sprintf("执行失败: %v 个", errorCount)
			messageType = "finalError"
		}
		if successCount > 0 {
			message += fmt.Sprintf("执行成功: %v 个", successCount)
		}
		if message == "" {
			message = "finished"
		}
		stream.Send(&ypb.ExportYakScriptLocalResponse{
			Progress:    1,
			Message:     message,
			MessageType: messageType,
			OutputDir:   dir,
		})
	}()

	for v := range scripts {
		if total > 0 {
			progress = float64(count) / float64(total)
		}
		count++
		outputPluginDir := v.ScriptName
		dirRet, err := s.ExportYakPluginBatch(v, req.GetOutputDir(), ReplaceString(outputPluginDir))
		if len(req.GetYakScriptIds()) == 1 {
			dir = dirRet
		}
		if count%100 == 0 {
			time.Sleep(1 * time.Second)
		}
		if err != nil {
			stream.Send(&ypb.ExportYakScriptLocalResponse{
				Progress:    progress,
				Message:     fmt.Sprintf("export [%s] failed: %s", v.ScriptName, err.Error()),
				MessageType: "error",
			})
			errorCount++
		} else {
			stream.Send(&ypb.ExportYakScriptLocalResponse{
				Progress:    progress,
				Message:     fmt.Sprintf("export [%s] success", v.ScriptName),
				MessageType: "success",
			})
			successCount++
		}
	}

	return nil
}
