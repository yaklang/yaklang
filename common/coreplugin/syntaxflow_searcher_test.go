package coreplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func sendSSAURL(t *testing.T, local ypb.YakClient, resultID int, programName, kind string) []*ypb.YakURLResource {
	url := &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema:   "syntaxflow",
			Location: programName,
			Path:     fmt.Sprintf("/%s", kind),
			Query: []*ypb.KVPair{
				{
					// get from database
					Key:   "result_id",
					Value: strconv.Itoa(resultID),
				},
			},
		},
	}
	res, err := local.RequestYakURL(context.Background(), url)
	require.NoError(t, err)
	t.Log("checkHandler in database query ")
	resultIDRes := res.Resources[len(res.Resources)-1]
	require.Equal(t, resultIDRes.ResourceType, "result_id")
	require.Equal(t, resultIDRes.VerboseType, "result_id")
	// got result
	gotResultID := resultIDRes.ResourceName
	require.Equal(t, strconv.Itoa(resultID), gotResultID)

	return res.Resources[:len(res.Resources)-1]
}

func getRangeText(res *ypb.YakURLResource, client ypb.YakClient) (string, error) {
	var rng ssaapi.CodeRange
	var source string

	for _, res := range res.Extra {
		if res.Key == "code_range" {
			if err := json.Unmarshal([]byte(res.Value), &rng); err != nil {
				return "", err
			}
		}
		if res.Key == "source" {
			source = res.Value
		}
	}

	// check rng file url
	if rng.URL == "" {
		return "", fmt.Errorf("no file url in code range")
	}
	if response, err := client.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema: "ssadb",
			Path:   rng.URL,
		},
	}); err != nil {
		return "", utils.Wrapf(err, "request file url %s failed", rng.URL)
	} else {
		spew.Dump(response)
	}

	// get source code by range
	editor := memedit.NewMemEditor(source)
	got := editor.GetTextFromPositionInt(
		int(rng.StartLine-rng.SourceCodeLine), int(rng.StartColumn),
		int(rng.EndLine-rng.SourceCodeLine), int(rng.EndColumn),
	)
	return got, nil
}

type sfSearch struct {
	fs    filesys_interface.FileSystem
	local ypb.YakClient
	code  string

	progName string

	t *testing.T
}

func NewSfSearch(fs filesys_interface.FileSystem, t *testing.T, opt ...ssaconfig.Option) *sfSearch {
	progName := uuid.NewString()
	client, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)
	{

		opt = append(opt,
			ssaapi.WithFileSystem(fs),
			ssaapi.WithProgramName(progName),
		)
		_, err := ssaapi.ParseProject(opt...)
		require.NoError(t, err)
		t.Cleanup(func() {
			log.Infof("delete program: %v", progName)
			ssadb.DeleteProgram(ssadb.GetDB(), progName)
		})
	}
	_, err = ssaapi.FromDatabase(progName)
	require.NoError(t, err)

	pluginName := "SyntaxFlow Searcher"
	initDB.Do(func() {
		yakit.InitialDatabase()
	})
	codeBytes := GetCorePluginData(pluginName)
	require.NotNilf(t, codeBytes, "无法从bindata获取: %v", pluginName)

	return &sfSearch{
		fs:       fs,
		local:    client,
		progName: progName,
		code:     string(codeBytes),
		t:        t,
	}
}

func (s *sfSearch) RunSearch(kind, input string, fuzz bool) int {
	var execParams []*ypb.KVPair

	execParams = []*ypb.KVPair{
		{
			Key:   "kind",
			Value: kind,
		},
		{
			Key:   "rule",
			Value: input,
		},
		{
			Key:   "progName",
			Value: s.progName,
		},
	}
	if fuzz {
		execParams = append(execParams, &ypb.KVPair{
			Key:   "fuzz",
			Value: strconv.FormatBool(fuzz),
		},
		)
	}
	stream, err := s.local.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       s.code,
		PluginType: "yak",
		ExecParams: execParams})
	require.NoError(s.t, err)
	resultId := -1
	result := new(msg)
	for {
		exec, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(s.t, err)
		}
		if exec.IsMessage {
			rawMsg := exec.GetMessage()
			fmt.Println("raw msg: ", string(rawMsg))
			json.Unmarshal(rawMsg, &result)
			if result.Content.Level == "json" && result.Content.Data != "" {
				id, err := strconv.Atoi(result.Content.Data)
				if err != nil {
					log.Errorf("invalid result id: %v", string(rawMsg))
					continue
				}
				resultId = id
				break
			}
		}
	}
	return resultId
}

func (s *sfSearch) Check(t *testing.T, kind string, resultId int, want map[string][]string) {
	rets := sendSSAURL(t, s.local, resultId, s.progName, kind)
	spew.Dump(rets)
	got := map[string][]string{}
	for _, ret := range rets {
		if ret.ResourceType != "value" {
			continue
		}
		key := ret.ResourceName
		source, err := getRangeText(ret, s.local)
		require.NoError(t, err)
		got[key] = append(got[key], source)
	}
	spew.Dump("got:", got)
	spew.Dump("want:", want)
	require.Equal(t, len(want), len(got))
	for name, wantSources := range want {
		gotSources, ok := got[name]
		require.True(t, ok, "not found: %v", name)

		// 比较两个 string slice 是否元素一致（不要求顺序）
		require.ElementsMatch(t, wantSources, gotSources, "mismatch for %v", name)
	}
}

func (s *sfSearch) SearchAndCheck(t *testing.T, kind, input string, fuzz bool, want map[string][]string) {
	resultId := s.RunSearch(kind, input, fuzz)
	s.Check(t, kind, resultId, want)
}

func TestSSASearch(t *testing.T) {
	fs := filesys.NewVirtualFs()
	code1 := `<?php
$b = "funcA(";
function funcA(){}
funcA(111);

function funcAxxxx() {}
function yyyyfuncAxxxx() {}
`
	fs.AddFile("/var/www/html/1.php", code1)
	code3 := `<?php
funcA(222);
`
	fs.AddFile("/var/www/html/funcA.php", code3)

	s := NewSfSearch(fs, t, ssaapi.WithLanguage(ssaconfig.PHP))

	t.Run("check all funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "all", "funcA", false, map[string][]string{
			"Function-funcA":           {"function funcA(){}"},
			"Undefined-funcA":          {"funcA"},
			"Function-funcA(111)":      {"funcA(111)"},
			"Undefined-funcA(222)":     {"funcA(222)"},
			`"var/www/html/funcA.php"`: {code3},
		})
		s.SearchAndCheck(t, "all", "funcA", true, map[string][]string{
			"Function-funcA":           {"function funcA(){}"},
			"Function-funcAxxxx":       {"function funcAxxxx() {}"},
			"Function-yyyyfuncAxxxx":   {"function yyyyfuncAxxxx() {}"},
			"Undefined-funcA":          {"funcA"},
			`"funcA("`:                 {"funcA("},
			"Function-funcA(111)":      {"funcA(111)"},
			"Undefined-funcA(222)":     {"funcA(222)"},
			`"var/www/html/funcA.php"`: {code3},
		})
	})

	t.Run("check symbol funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "symbol", "funcA", false, map[string][]string{
			"Function-funcA":  {"function funcA(){}"},
			"Undefined-funcA": {"funcA"},
		})
	})

	t.Run("check function funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "function", "funcA", false, map[string][]string{
			"Function-funcA": {"function funcA(){}"},
		})
	})

	t.Run("check function funcA with fuzz", func(t *testing.T) {
		s.SearchAndCheck(t, "function", "funcA", true, map[string][]string{
			"Function-funcA":         {"function funcA(){}"},
			"Function-funcAxxxx":     {"function funcAxxxx() {}"},
			"Function-yyyyfuncAxxxx": {"function yyyyfuncAxxxx() {}"},
		})
	})

	t.Run("check call funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "call", "funcA", false, map[string][]string{
			"Function-funcA(111)":  {"funcA(111)"},
			"Undefined-funcA(222)": {"funcA(222)"},
		})
	})

	t.Run("check file funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "file", "funcA", false, map[string][]string{
			`"var/www/html/funcA.php"`: {code3},
		})
	})

	t.Run("check const funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "const", "funcA", true, map[string][]string{
			`"funcA("`: {"funcA("},
		})
		s.SearchAndCheck(t, "const", "funcA", false, map[string][]string{})
	})

	t.Run("check  call", func(t *testing.T) {
		s.SearchAndCheck(t, "call", "funcA(111)", false, map[string][]string{
			"Function-funcA(111)":  {"funcA(111)"},
			"Undefined-funcA(222)": {"funcA(222)"},
		})
	})
}

func TestSSASearch_OnceSearch_MultipleQueryKind(t *testing.T) {
	fs := filesys.NewVirtualFs()
	code1 := `<?php
$b = "funcA(";
function funcA(){}
funcA(111);

function funcAxxxx() {}
function yyyyfuncAxxxx() {}
`
	fs.AddFile("/var/www/html/1.php", code1)
	code3 := `<?php
funcA(222);
`
	fs.AddFile("/var/www/html/funcA.php", code3)

	s := NewSfSearch(fs, t, ssaapi.WithLanguage(ssaconfig.PHP))

	// search
	_ = s
	result := s.RunSearch("all", "funcA", true)

	// check all
	s.Check(t, "all", result, map[string][]string{
		"Function-funcA":           {"function funcA(){}"},
		"Function-funcAxxxx":       {"function funcAxxxx() {}"},
		"Function-yyyyfuncAxxxx":   {"function yyyyfuncAxxxx() {}"},
		"Undefined-funcA":          {"funcA"},
		`"funcA("`:                 {"funcA("},
		"Function-funcA(111)":      {"funcA(111)"},
		"Undefined-funcA(222)":     {"funcA(222)"},
		`"var/www/html/funcA.php"`: {code3},
	})

	// check file
	s.Check(t, "file", result, map[string][]string{
		`"var/www/html/funcA.php"`: {code3},
	})

	// check function
	s.Check(t, "function", result, map[string][]string{
		"Function-funcA":         {"function funcA(){}"},
		"Function-funcAxxxx":     {"function funcAxxxx() {}"},
		"Function-yyyyfuncAxxxx": {"function yyyyfuncAxxxx() {}"},
	})

	// check symbol
	s.Check(t, "symbol", result, map[string][]string{
		"Function-funcA":         {"function funcA(){}"},
		"Function-funcAxxxx":     {"function funcAxxxx() {}"},
		"Function-yyyyfuncAxxxx": {"function yyyyfuncAxxxx() {}"},
		"Undefined-funcA":        {"funcA"},
	})

	// check call
	s.Check(t, "call", result, map[string][]string{
		"Function-funcA(111)":  {"funcA(111)"},
		"Undefined-funcA(222)": {"funcA(222)"},
	})

	// check const
	s.Check(t, "const", result, map[string][]string{
		`"funcA("`: {"funcA("},
	})

}

func TestSSASearch_MultipleSearch_HitCache(t *testing.T) {
	fs := filesys.NewVirtualFs()
	code1 := `<?php
$b = "funcA(";
function funcA(){}
funcA(111);

function funcAxxxx() {}
function yyyyfuncAxxxx() {}
`
	fs.AddFile("/var/www/html/1.php", code1)
	code3 := `<?php
funcA(222);
`
	fs.AddFile("/var/www/html/funcA.php", code3)

	s := NewSfSearch(fs, t, ssaapi.WithLanguage(ssaconfig.PHP))

	// search
	result := s.RunSearch("all", "funcA", true)
	log.Infof("result: %v", result)

	t.Run("check database cache ", func(t *testing.T) {
		// check database cache
		key := fmt.Sprint([]any{s.progName, "all", "funcA", true})
		log.Infof("key: %s", key)
		res, err := s.local.GetKey(context.Background(), &ypb.GetKeyRequest{
			Key: key,
		})
		require.NoError(t, err)
		got := res.GetValue()
		log.Infof("got: %v", got)
		gotResult, err := strconv.Atoi(got)
		require.NoError(t, err)
		require.Equal(t, result, gotResult)
	})

	t.Run("check search again", func(t *testing.T) {
		// search again
		resultGot := s.RunSearch("all", "funcA", true)
		require.Equal(t, result, resultGot)
	})

	t.Run("check search again with different kind: should same id", func(t *testing.T) {
		// search
		// other kind can get "all" kind cache
		resultGot := s.RunSearch("file", "funcA", true)
		require.Equal(t, result, resultGot)
	})

	t.Run("all kind con't got other kind", func(t *testing.T) {
		str := strings.ReplaceAll(uuid.NewString(), "-", "")
		resultWant := s.RunSearch("file", str, true)
		resultGot := s.RunSearch("all", str, true)
		require.NotEqual(t, resultWant, resultGot)
	})

	t.Run("negative: delete result but still in cache", func(t *testing.T) {
		str := strings.ReplaceAll(uuid.NewString(), "-", "")
		// search
		result1 := s.RunSearch("all", str, true)
		log.Infof("result: %v", result1)
		ssadb.DeleteResultByID(uint(result1))
		// search again
		resultGot := s.RunSearch("all", str, true)
		require.NotEqual(t, result1, resultGot)
	})

}

func TestSSASearch_Annotation_Syntax(t *testing.T) {
	fs := filesys.NewVirtualFs()
	code1 := `
package net.mingsoft.mdiy.action;

import cn.hutool.core.map.CaseInsensitiveMap;
import cn.hutool.core.util.ObjectUtil;
import com.baomidou.mybatisplus.core.conditions.Wrapper;
import com.baomidou.mybatisplus.core.conditions.query.LambdaQueryWrapper;
import io.swagger.annotations.Api;
import io.swagger.annotations.ApiImplicitParam;
import io.swagger.annotations.ApiImplicitParams;
import io.swagger.annotations.ApiOperation;
import java.util.Map;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import net.mingsoft.base.entity.ResultData;
import net.mingsoft.base.exception.BusinessException;
import net.mingsoft.basic.annotation.LogAnn;
import net.mingsoft.basic.constant.e.BusinessTypeEnum;
import net.mingsoft.basic.util.BasicUtil;
import net.mingsoft.mdiy.biz.IModelBiz;
import net.mingsoft.mdiy.biz.IModelDataBiz;
import net.mingsoft.mdiy.constant.e.ModelCustomTypeEnum;
import net.mingsoft.mdiy.entity.ModelEntity;
import org.apache.commons.lang3.StringUtils;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Controller;
import org.springframework.ui.ModelMap;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestMethod;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.ResponseBody;
import springfox.documentation.annotations.ApiIgnore;

@Api(
    tags = {"后端-自定义模块接口"}
)
@Controller
@RequestMapping({"/${ms.manager.path}/mdiy/form/data"})
public class FormDataAction extends BaseAction {
    @Autowired
    private IModelDataBiz modelDataBiz;
    @Autowired
    private IModelBiz modelBiz;

    public FormDataAction() {
    }

    @ApiIgnore
    @GetMapping({"/index"})
    public String index(HttpServletResponse response, HttpServletRequest request, @ApiIgnore ModelMap model) {
        String modelId = BasicUtil.getString("modelId");
        ModelEntity modelEntity = (ModelEntity)this.modelBiz.getOne((Wrapper)((LambdaQueryWrapper)(new LambdaQueryWrapper()).eq(ModelEntity::getId, modelId)).eq(ModelEntity::getModelCustomType, ModelCustomTypeEnum.FORM.getLabel()));
        if (modelEntity == null) {
            throw new BusinessException(this.getResString("err.not.exist", new String[]{this.getResString("model.id")}));
        } else {
            return "/mdiy/form/data/index";
        }
    }

    @ApiIgnore
    @GetMapping({"/form"})
    public String form(HttpServletResponse response, HttpServletRequest request, @ApiIgnore ModelMap model) {
        String businessForm = BasicUtil.getString("businessUrl");
        return StringUtils.isNotBlank(businessForm) ? businessForm : "/mdiy/form/data/form";
    }

    @ApiOperation("提供后台查询自定义表单提交数据")
    @ApiImplicitParams({@ApiImplicitParam(
    name = "modelId",
    value = "模型编号",
    required = true,
    paramType = "query"
), @ApiImplicitParam(
    name = "modelName",
    value = "模型名称",
    required = false,
    paramType = "query"
)})
    @RequestMapping(
        value = {"/queryData"},
        method = {RequestMethod.GET, RequestMethod.POST}
    )
    @ResponseBody
    public ResultData queryData(HttpServletRequest request, HttpServletResponse response) {
        Map<String, Object> map = BasicUtil.assemblyRequestMap();
        LambdaQueryWrapper<ModelEntity> wrapper = new LambdaQueryWrapper();
        ((LambdaQueryWrapper)wrapper.eq(ModelEntity::getId, map.get("modelId"))).eq(ModelEntity::getModelCustomType, ModelCustomTypeEnum.FORM.getLabel());
        ModelEntity modelEntity = (ModelEntity)this.modelBiz.getOne(wrapper, false);
        if (modelEntity == null) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("model.id")}));
        } else if (!this.hasPermissions("mdiy:formData:view", "mdiy:formData:" + modelEntity.getId() + ":view")) {
            return ResultData.build().error("没有权限!");
        } else {
            map.putIfAbsent("order", "desc");
            map.putIfAbsent("orderBy", "id");
            return ResultData.build().success(this.modelDataBiz.queryDiyFormData(modelEntity.getId(), map));
        }
    }

    @ApiOperation("提供后台查询自定义表单提交数据")
    @ApiImplicitParams({@ApiImplicitParam(
    name = "modelId",
    value = "模型编号",
    required = true,
    paramType = "query"
), @ApiImplicitParam(
    name = "id",
    value = "主键编号",
    required = true,
    paramType = "query"
)})
    @GetMapping({"/getData"})
    @ResponseBody
    public ResultData getData(HttpServletRequest request, HttpServletResponse response) {
        String modelId = BasicUtil.getString("modelId");
        String id = BasicUtil.getString("id");
        LambdaQueryWrapper<ModelEntity> wrapper = new LambdaQueryWrapper();
        wrapper.eq(ModelEntity::getId, modelId);
        wrapper.eq(ModelEntity::getModelCustomType, ModelCustomTypeEnum.FORM.getLabel());
        ModelEntity modelEntity = (ModelEntity)this.modelBiz.getOne(wrapper, false);
        if (modelEntity == null) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("model.id")}));
        } else if (StringUtils.isEmpty(id)) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("id")}));
        } else {
            Object object = this.modelDataBiz.getFormData(modelEntity.getId(), id);
            return ObjectUtil.isNotNull(object) ? ResultData.build().success(object) : ResultData.build().error();
        }
    }

    @ApiOperation("自定义业务数据保存")
    @ApiImplicitParams({@ApiImplicitParam(
    name = "modelName",
    value = "业务模型名称",
    required = true,
    paramType = "query"
), @ApiImplicitParam(
    name = "modelId",
    value = "业务模型Id",
    required = false,
    paramType = "query"
)})
    @LogAnn(
        title = "新增自定义业务数据",
        businessType = BusinessTypeEnum.INSERT
    )
    @PostMapping({"save"})
    @ResponseBody
    public ResultData save(HttpServletRequest request, HttpServletResponse response) {
        Map<String, Object> map = BasicUtil.assemblyRequestMap();
        CaseInsensitiveMap<String, Object> caseIgnoreMap = new CaseInsensitiveMap(map);
        String modelName = BasicUtil.getString("modelName");
        String modelId = BasicUtil.getString("modelId");
        if (StringUtils.isBlank(modelName) && StringUtils.isBlank(modelId)) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("model.id")})).code("mdiyErrCode");
        } else {
            LambdaQueryWrapper<ModelEntity> wrapper = new LambdaQueryWrapper();
            ((LambdaQueryWrapper)((LambdaQueryWrapper)wrapper.eq(StringUtils.isNotEmpty(modelName), ModelEntity::getModelName, modelName)).eq(StringUtils.isNotEmpty(modelId), ModelEntity::getId, modelId)).eq(ModelEntity::getModelCustomType, ModelCustomTypeEnum.FORM.getLabel());
            ModelEntity modelEntity = (ModelEntity)this.modelBiz.getOne(wrapper, true);
            if (modelEntity == null) {
                return ResultData.build().error(this.getResString("err.not.exist", new String[]{this.getResString("model.name")})).code("mdiyErrCode");
            } else if (!this.hasPermissions("mdiy:formData:save", "mdiy:formData:" + modelEntity.getId() + ":save")) {
                return ResultData.build().error("没有权限!").code("mdiyErrCode");
            } else {
                return this.modelDataBiz.saveDiyFormData(modelEntity.getId(), caseIgnoreMap) ? ResultData.build().success() : ResultData.build().error(this.getResString("err.error", new String[]{this.getResString("model.id")})).code("mdiyErrCode");
            }
        }
    }

    @ApiOperation("更新自定义业务数据")
    @ApiImplicitParam(
        name = "modelId",
        value = "模型编号",
        required = true,
        paramType = "query"
    )
    @LogAnn(
        title = "更新自定义业务数据",
        businessType = BusinessTypeEnum.UPDATE
    )
    @PostMapping({"update"})
    @ResponseBody
    public ResultData update(HttpServletRequest request, HttpServletResponse response) {
        Map<String, Object> map = BasicUtil.assemblyRequestMap();
        CaseInsensitiveMap<String, Object> caseIgnoreMap = new CaseInsensitiveMap(map);
        String modelId = caseIgnoreMap.get("modelId").toString();
        if (StringUtils.isBlank(modelId)) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("model.id")})).code("mdiyErrCode");
        } else {
            ModelEntity modelEntity = this.modelBiz.getById(modelId);
            if (!this.hasPermissions("mdiy:formData:update", "mdiy:formData:" + modelEntity.getId() + ":update")) {
                return ResultData.build().error("没有权限!").code("mdiyErrCode");
            } else {
                return this.modelDataBiz.updateDiyFormData(modelEntity, caseIgnoreMap) ? ResultData.build().success() : ResultData.build().error(this.getResString("err.error", new String[]{this.getResString("model.id")})).code("mdiyErrCode");
            }
        }
    }

    @ApiOperation("批量删除自定义业务数据接口")
    @LogAnn(
        title = "批量删除自定义业务数据接口",
        businessType = BusinessTypeEnum.DELETE
    )
    @PostMapping({"delete"})
    @ResponseBody
    public ResultData delete(@RequestParam("modelId") String modelId, HttpServletResponse response, HttpServletRequest request) {
        String ids = BasicUtil.getString("ids");
        if (StringUtils.isBlank(ids)) {
            return ResultData.build().error(this.getResString("err.error", new String[]{this.getResString("id")}));
        } else if (StringUtils.isBlank(modelId)) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("model.id")}));
        } else {
            ModelEntity modelEntity = this.modelBiz.getById(modelId);
            if (!this.hasPermissions("mdiy:formData:del", "mdiy:formData:" + modelEntity.getId() + ":del")) {
                return ResultData.build().error("没有权限!");
            } else {
                String[] _ids = ids.split(",");
                String[] var7 = _ids;
                int var8 = _ids.length;

                for(int var9 = 0; var9 < var8; ++var9) {
                    String id = var7[var9];
                    this.modelDataBiz.deleteQueryDiyFormData(id, modelId);
                }

                return ResultData.build().success();
            }
        }
    }
}
`
	memEditor := memedit.NewMemEditor(code1)
	fs.AddFile("/var/java/test.java", code1)
	s := NewSfSearch(fs, t, ssaapi.WithLanguage(ssaconfig.JAVA))
	// search
	s.SearchAndCheck(t, "function", "@RequestMapping", false, map[string][]string{
		"Function-FormDataAction.queryData": {memEditor.GetTextFromPositionInt(86, 12, 100, 6)}, // annotated func
	})

	s.SearchAndCheck(t, "symbol", "@RequestMapping", false, map[string][]string{
		"RequestMapping": {memEditor.GetTextFromPositionInt(30, 1, 30, 63)},                                                 // import string
		"make(any)":      {memEditor.GetTextFromPositionInt(40, 1, 40, 56), memEditor.GetTextFromPositionInt(81, 5, 84, 6)}, // annotation mark in class
	})

	t.Run("check search again but fuzz", func(t *testing.T) {
		s.SearchAndCheck(t, "function", "@RequestMap", true, map[string][]string{
			"Function-FormDataAction.queryData": {memEditor.GetTextFromPositionInt(86, 12, 100, 6)}, // annotated func
		})

		s.SearchAndCheck(t, "symbol", "@RequestMap", true, map[string][]string{
			"RequestMapping": {memEditor.GetTextFromPositionInt(30, 1, 30, 63)},                                                 // import string
			"make(any)":      {memEditor.GetTextFromPositionInt(40, 1, 40, 56), memEditor.GetTextFromPositionInt(81, 5, 84, 6)}, // annotation mark in class
		})
	})

}

func TestSSASearch_Mybatis_Syntax(t *testing.T) {
	fs := filesys.NewVirtualFs()
	code1 := `<?xml version="1.0" encoding="UTF-8" ?>
	<!DOCTYPE mapper PUBLIC "-//mybatis.org//DTD Mapper 3.0//EN" "http://mybatis.org/dtd/mybatis-3-mapper.dtd" >
	<mapper namespace="net.mingsoft.cms.dao.IContentDao">
	
		<resultMap id="resultMap" type="net.mingsoft.cms.entity.ContentEntity">
				<id column="id" property="id" /><!--编号 -->
					<result column="content_title" property="contentTitle" /><!--文章标题 -->
					<result column="content_short_title" property="contentShortTitle" /><!--文章副标题 -->
					<result column="category_id" property="categoryId" /><!--所属栏目 -->
					<result column="content_type" property="contentType" /><!--文章类型 -->
					<result column="content_display" property="contentDisplay" /><!--是否显示 -->
					<result column="content_author" property="contentAuthor" /><!--文章作者 -->
					<result column="content_source" property="contentSource" /><!--文章来源 -->
					<result column="content_datetime" property="contentDatetime" /><!--发布时间 -->
					<result column="content_tags" property="contentTags" /><!--文章标签 -->
					<result column="content_sort" property="contentSort" /><!--自定义顺序 -->
					<result column="content_img" property="contentImg" /><!--文章缩略图 -->
					<result column="content_description" property="contentDescription" /><!--描述 -->
					<result column="content_keyword" property="contentKeyword" /><!--关键字 -->
					<result column="content_details" property="contentDetails" /><!--文章内容 -->
					<result column="content_out_link" property="contentOutLink" /><!--文章跳转链接地址 -->
					<result column="content_hit" property="contentHit" /><!--点击次数 -->
					<result column="create_by" property="createBy" /><!--创建人 -->
					<result column="create_date" property="createDate" /><!--创建时间 -->
					<result column="update_by" property="updateBy" /><!--修改人 -->
					<result column="update_date" property="updateDate" /><!--修改时间 -->
					<result column="del" property="del" /><!--删除标记 -->
		</resultMap>
	
		<resultMap id="resultContentMap" type="net.mingsoft.cms.bean.ContentBean">
			<id column="id" property="id" /><!--编号 -->
			<result column="content_title" property="contentTitle" /><!--文章标题 -->
			<result column="content_short_title" property="contentShortTitle" /><!--文章副标题 -->
			<result column="category_id" property="categoryId" /><!--所属栏目 -->
			<result column="content_type" property="contentType" /><!--文章类型 -->
			<result column="content_display" property="contentDisplay" /><!--是否显示 -->
			<result column="content_author" property="contentAuthor" /><!--文章作者 -->
			<result column="content_source" property="contentSource" /><!--文章来源 -->
			<result column="content_datetime" property="contentDatetime" /><!--发布时间 -->
			<result column="content_tags" property="contentTags" /><!--文章标签 -->
			<result column="content_sort" property="contentSort" /><!--自定义顺序 -->
			<result column="content_img" property="contentImg" /><!--文章缩略图 -->
			<result column="content_description" property="contentDescription" /><!--描述 -->
			<result column="content_keyword" property="contentKeyword" /><!--关键字 -->
			<result column="content_details" property="contentDetails" /><!--文章内容 -->
			<result column="content_out_link" property="contentOutLink" /><!--文章跳转链接地址 -->
	<!--		<result column="static_url" property="staticUrl" />&lt;!&ndash;静态地址 &ndash;&gt;-->
			<result column="content_hit" property="contentHit" /><!--点击次数 -->
			<result column="create_by" property="createBy" /><!--创建人 -->
			<result column="create_date" property="createDate" /><!--创建时间 -->
			<result column="update_by" property="updateBy" /><!--修改人 -->
			<result column="update_date" property="updateDate" /><!--修改时间 -->
			<result column="del" property="del" /><!--删除标记 -->
		</resultMap>
	
		<resultMap id="resultBean" type="net.mingsoft.cms.bean.CategoryBean">
			<id column="id" property="id" /><!--编号 -->
			<id column="article_Id" property="articleId" /><!--编号 -->
			<result column="content_update_date" property="contentUpdateDate" /><!--文章更新时间-->
			<result column="category_title" property="categoryTitle" /><!--栏目管理名称 -->
			<result column="category_short_title" property="categoryShortTitle" /><!--栏目副标题 -->
			<result column="category_id" property="categoryId" /><!--所属栏目 -->
			<result column="category_type" property="categoryType" /><!--栏目管理属性 -->
			<result column="category_sort" property="categorySort" /><!--自定义顺序 -->
			<result column="category_list_url" property="categoryListUrl" /><!--列表模板 -->
			<result column="category_url" property="categoryUrl" /><!--内容模板 -->
			<result column="category_keyword" property="categoryKeyword" /><!--栏目管理关键字 -->
			<result column="category_descrip" property="categoryDescrip" /><!--栏目管理描述 -->
			<result column="category_img" property="categoryImg" /><!--缩略图 -->
			<result column="category_diy_url" property="categoryDiyUrl" /><!--自定义链接 -->
			<result column="mdiy_model_id" property="mdiyModelId" /><!--栏目管理的内容模型id -->
			<result column="dict_id" property="dictId" /><!--字典对应编号 -->
			<result column="category_flag" property="categoryFlag" /><!--栏目属性 -->
			<result column="category_path" property="categoryPath" /><!--栏目路径 -->
			<result column="category_parent_ids" property="categoryParentIds" /><!--父类型编号 -->
			<result column="create_by" property="createBy" /><!--创建人 -->
			<result column="create_date" property="createDate" /><!--创建时间 -->
			<result column="update_by" property="updateBy" /><!--修改人 -->
			<result column="update_date" property="updateDate" /><!--修改时间 -->
			<result column="del" property="del" /><!--删除标记 -->
		</resultMap>
	
		<!--保存-->
		<insert id="saveEntity" useGeneratedKeys="true" keyProperty="id"
				parameterType="net.mingsoft.cms.entity.ContentEntity" >
			insert into cms_content
			<trim prefix="(" suffix=")" suffixOverrides=",">
					<if test="contentTitle != null and contentTitle != ''">content_title,</if>
					<if test="contentShortTitle != null and contentShortTitle != ''">content_short_title,</if>
					<if test="categoryId != null and categoryId != ''">category_id,</if>
					<if test="contentType != null ">content_type,</if>
					<if test="contentDisplay != null and contentDisplay != ''">content_display,</if>
					<if test="contentAuthor != null and contentAuthor != ''">content_author,</if>
					<if test="contentSource != null and contentSource != ''">content_source,</if>
					<if test="contentDatetime != null">content_datetime,</if>
					<if test="contentSort != null">content_sort,</if>
					<if test="contentTags != null and contentTags != ''">content_tags,</if>
					<if test="contentImg != null and contentImg != ''">content_img,</if>
					<if test="contentDescription != null and contentDescription != ''">content_description,</if>
					<if test="contentKeyword != null and contentKeyword != ''">content_keyword,</if>
					<if test="contentDetails != null and contentDetails != ''">content_details,</if>
					<if test="contentOutLink != null and contentOutLink != ''">content_out_link,</if>
					<if test="contentHit != null">content_hit,</if>
					<if test="createBy &gt; 0">create_by,</if>
					<if test="createDate != null">create_date,</if>
					<if test="updateBy &gt; 0">update_by,</if>
					<if test="updateDate != null">update_date,</if>
					<if test="del != null">del,</if>
			</trim>
			<trim prefix="values (" suffix=")" suffixOverrides=",">
					<if test="contentTitle != null and contentTitle != ''">#{contentTitle},</if>
					<if test="contentShortTitle != null and contentShortTitle != ''">#{contentShortTitle},</if>
					<if test="categoryId != null and categoryId != ''">#{categoryId},</if>
					<if test="contentType != null ">#{contentType},</if>
					<if test="contentDisplay != null and contentDisplay != ''">#{contentDisplay},</if>
					<if test="contentAuthor != null and contentAuthor != ''">#{contentAuthor},</if>
					<if test="contentSource != null and contentSource != ''">#{contentSource},</if>
					<if test="contentDatetime != null">#{contentDatetime},</if>
					<if test="contentSort != null">#{contentSort},</if>
					<if test="contentTags != null and contentTags != ''">#{contentTags},</if>
					<if test="contentImg != null and contentImg != ''">#{contentImg},</if>
					<if test="contentDescription != null and contentDescription != ''">#{contentDescription},</if>
					<if test="contentKeyword != null and contentKeyword != ''">#{contentKeyword},</if>
					<if test="contentDetails != null and contentDetails != ''">#{contentDetails},</if>
					<if test="contentUrl != null and contentUrl != ''">#{contentUrl},</if>
					<if test="contentHit != null">#{contentHit},</if>
					<if test="createBy &gt; 0">#{createBy},</if>
					<if test="createDate != null">#{createDate},</if>
					<if test="updateBy &gt; 0">#{updateBy},</if>
					<if test="updateDate != null">#{updateDate},</if>
					<if test="del != null">#{del},</if>
			</trim>
		</insert>
	
			<!--更新-->
			<update id="updateEntity" parameterType="net.mingsoft.cms.entity.ContentEntity">
				update cms_content
				<set>
					<if test="contentTitle != null and contentTitle != ''">content_title=#{contentTitle},</if>
					<if test="contentShortTitle != null and contentShortTitle != ''">content_short_title=#{contentShortTitle},</if>
					<if test="categoryId != null and categoryId != ''">category_id=#{categoryId},</if>
					<if test="contentType != null ">content_type=#{contentType},</if>
					<if test="contentDisplay != null and contentDisplay != ''">content_display=#{contentDisplay},</if>
					<if test="contentAuthor != null ">content_author=#{contentAuthor},</if>
					<if test="contentSource != null ">content_source=#{contentSource},</if>
					<if test="contentDatetime != null">content_datetime=#{contentDatetime},</if>
					<if test="contentSort != null">content_sort=#{contentSort},</if>
					<if test="contentTags != null and contentTags != ''">content_tags=#{contentTags},</if>
					<if test="contentImg != null and contentImg != ''">content_img=#{contentImg},</if>
					<if test="contentDescription != null ">content_description=#{contentDescription},</if>
					<if test="contentKeyword != null ">content_keyword=#{contentKeyword},</if>
					<if test="contentDetails != null ">content_details=#{contentDetails},</if>
					<if test="contentOutLink != null and contentOutLink != ''">content_out_link=#{contentOutLink},</if>
					<if test="contentHit != null">content_hit=#{contentHit},</if>
					<if test="createBy &gt; 0">create_by=#{createBy},</if>
					<if test="createDate != null">create_date=#{createDate},</if>
					<if test="updateBy &gt; 0">update_by=#{updateBy},</if>
					<if test="updateDate != null">update_date=#{updateDate},</if>
					<if test="del != null">del=#{del},</if>
				</set>
				where id = #{id}
			</update>
	
			<!--根据id获取-->
			<select id="getEntity" resultMap="resultMap" parameterType="int">
				select * from cms_content where id=#{id} and del=0
			</select>
	
			<!--根据实体获取-->
			<select id="getByEntity" resultMap="resultMap" parameterType="net.mingsoft.cms.entity.ContentEntity">
				select * from cms_content
				<where>
					del=0
					<if test="contentTitle != null and contentTitle != ''">and  content_title like CONCAT(CONCAT('%',#{contentTitle}),'%')</if>
					<if test="contentShortTitle != null and contentShortTitle != ''">and  content_short_title like CONCAT(CONCAT('%',#{contentShortTitle}),'%')</if>
					<if test="categoryId != null and categoryId != ''">and category_id=#{categoryId}</if>
					<if test="contentType != null and contentType != ''">and content_type=#{contentType}</if>
					<if test="contentDisplay != null and contentDisplay != ''">and content_display=#{contentDisplay}</if>
					<if test="contentAuthor != null and contentAuthor != ''">and content_author=#{contentAuthor}</if>
					<if test="contentSource != null and contentSource != ''">and content_source=#{contentSource}</if>
					<if test="contentDatetime != null"> and content_datetime=#{contentDatetime} </if>
					<if test="contentSort != null"> and content_sort=#{contentSort} </if>
					<if test="contentTags != null and contentTags != ''">and content_tags=#{contentTags}</if>
					<if test="contentImg != null and contentImg != ''">and content_img=#{contentImg}</if>
					<if test="contentDescription != null and contentDescription != ''">and content_description=#{contentDescription}</if>
					<if test="contentKeyword != null and contentKeyword != ''">and content_keyword=#{contentKeyword}</if>
					<if test="contentDetails != null and contentDetails != ''">and content_details=#{contentDetails}</if>
					<if test="contentOutLink != null and contentOutLink != ''">and content_out_link=#{contentOutLink}</if>
					<if test="contentHit != null">and content_hit=#{contentHit}</if>
					<if test="createBy &gt; 0"> and create_by=#{createBy} </if>
					<if test="createDate != null"> and create_date=#{createDate} </if>
					<if test="updateBy &gt; 0"> and update_by=#{updateBy} </if>
					<if test="updateDate != null"> and update_date=#{updateDate} </if>
				</where>
			</select>
	
	
			<!--删除 防止脏数据-->
			<delete id="deleteEntity" parameterType="int">
				delete from cms_content where id=#{id}
			</delete>
	
			<!--删除 防止脏数据-->
			<delete id="deleteEntityByCategoryIds" >
				delete from cms_content
				<where>
					category_id in <foreach collection="ids" item="item" index="index"
								open="(" separator="," close=")">#{item}</foreach>
				</where>
			</delete>
	
			<!--批量删除-->
			<delete id="delete" >
				delete from cms_content
				<where>
					id in <foreach collection="ids" item="item" index="index"
											 open="(" separator="," close=")">#{item}</foreach>
				</where>
			</delete>
			<!--查询全部-->
			<select id="queryAll" resultMap="resultMap">
				select * from cms_content where del=0 order by id desc
			</select>
	
	
		<!--  查询文章,不包括单篇	-->
		<select id="queryContent" resultMap="resultContentMap">
			<!--,CONCAT('/html/',ct.app_id,category_path,'/',ct.id,'.html') AS static_url-->
			select ct.* from (
			select ct.*,cc.category_path from cms_content ct
			join cms_category cc on ct.category_id=cc.id
			<where>
				ct.del=0
				<if test="contentTitle != null and contentTitle != ''"> and  content_title like CONCAT(CONCAT('%',#{contentTitle}),'%')</if>
				<if test="contentShortTitle != null and contentShortTitle != ''"> and  content_short_title like CONCAT(CONCAT('%',#{contentShortTitle}),'%')</if>
				<if test="categoryId != null and categoryId != ''"> 	and (ct.category_id=#{categoryId} or ct.category_id in
					(select id FROM cms_category where find_in_set(#{categoryId},CATEGORY_PARENT_IDS)>0 and cms_category.category_type != '2'))</if>
				<if test="contentType != null and contentType != ''">
					and
					<foreach item="item" index="index" collection="contentType.split(',')" open="(" separator="or"
							 close=")">
						FIND_IN_SET(#{item},ct.content_type)>0
					</foreach>
				</if>
				<if test="contentDisplay != null and contentDisplay != ''"> and content_display=#{contentDisplay}</if>
				<if test="contentAuthor != null and contentAuthor != ''"> and content_author=#{contentAuthor}</if>
				<if test="contentSource != null and contentSource != ''"> and content_source=#{contentSource}</if>
				<if test="contentDatetime != null"> and content_datetime=#{contentDatetime} </if>
				<if test="contentSort != null"> and content_sort=#{contentSort} </if>
				<if test="contentTags != null and contentTags != ''">and content_tags=#{contentTags}</if>
				<if test="contentImg != null and contentImg != ''"> and content_img=#{contentImg}</if>
				<if test="contentDescription != null and contentDescription != ''"> and content_description=#{contentDescription}</if>
				<if test="contentKeyword != null and contentKeyword != ''"> and content_keyword=#{contentKeyword}</if>
				<if test="contentDetails != null and contentDetails != ''"> and content_details=#{contentDetails}</if>
				<if test="contentOutLink != null and contentOutLink != ''">and content_out_link=#{contentOutLink}</if>
				<if test="contentHit != null"> and content_hit=#{contentHit}</if>
				<if test="createBy &gt; 0"> and ct.create_by=#{createBy} </if>
				<if test="createDate != null"> and ct.create_date=#{createDate} </if>
				<if test="updateBy &gt; 0"> and ct.update_by=#{updateBy} </if>
				<if test="updateDate != null"> and update_date=#{updateDate} </if>
	
				<include refid="net.mingsoft.base.dao.IBaseDao.sqlWhere"></include>
			</where>
			)ct ORDER BY ct.content_datetime desc,content_sort desc
		</select>
		<!--条件查询-->
		<select id="query" resultMap="resultContentMap">
			<!--,CONCAT('/html/',ct.app_id,category_path,'/',ct.id,'.html') AS static_url-->
			select ct.* from (
			select ct.*,cc.category_path from cms_content ct
			join cms_category cc on ct.category_id=cc.id
			<where>
				ct.del=0
				<if test="contentTitle != null and contentTitle != ''"> and  content_title like CONCAT(CONCAT('%',#{contentTitle}),'%')</if>
				<if test="contentShortTitle != null and contentShortTitle != ''"> and  content_short_title like CONCAT(CONCAT('%',#{contentShortTitle}),'%')</if>
				<if test="categoryId != null and categoryId != ''"> 	and (ct.category_id=#{categoryId} or ct.category_id in
					(select id FROM cms_category where find_in_set(#{categoryId},CATEGORY_PARENT_IDS)>0))</if>
				<if test="contentType != null and contentType != ''">
					and
					<foreach item="item" index="index" collection="contentType.split(',')" open="(" separator="or"
							 close=")">
						FIND_IN_SET(#{item},ct.content_type)>0
					</foreach>
				</if>
				<if test="flag != null and flag != ''">
					and
					<foreach item="item" index="index" collection="flag.split(',')" open="(" separator="or"
							 close=")">
						FIND_IN_SET(#{item},ct.content_type)>0
					</foreach>
				</if>
				<if test="noflag != null and noflag != ''">
					and
					<foreach item="item" index="index" collection="noflag.split(',')" open="(" separator="and"
							 close=" or ct.content_type is null)">
						FIND_IN_SET(#{item},ct.content_type)=0
					</foreach>
				</if>
				<if test="contentDisplay != null and contentDisplay != ''"> and content_display=#{contentDisplay}</if>
				<if test="contentAuthor != null and contentAuthor != ''"> and content_author=#{contentAuthor}</if>
				<if test="contentSource != null and contentSource != ''"> and content_source=#{contentSource}</if>
				<if test="contentDatetime != null"> and content_datetime=#{contentDatetime} </if>
				<if test="contentSort != null"> and content_sort=#{contentSort} </if>
				<if test="contentImg != null and contentImg != ''"> and content_img=#{contentImg}</if>
				<if test="contentDescription != null and contentDescription != ''"> and content_description=#{contentDescription}</if>
				<if test="contentKeyword != null and contentKeyword != ''"> and content_keyword=#{contentKeyword}</if>
				<if test="contentDetails != null and contentDetails != ''"> and content_details=#{contentDetails}</if>
				<if test="contentOutLink != null and contentOutLink != ''">and content_out_link=#{contentOutLink}</if>
				<if test="contentHit != null"> and content_hit=#{contentHit}</if>
				<if test="createBy &gt; 0"> and ct.create_by=#{createBy} </if>
				<if test="createDate != null"> and ct.create_date=#{createDate} </if>
				<if test="updateBy &gt; 0"> and ct.update_by=#{updateBy} </if>
				<if test="updateDate != null"> and update_date=#{updateDate} </if>
				<include refid="net.mingsoft.base.dao.IBaseDao.sqlWhere"></include>
			</where>
			)ct ORDER BY ct.content_datetime desc,content_sort desc
		</select>
	
		<!-- 根据站点编号、开始、结束时间和栏目编号查询文章编号集合 -->
		<select id="queryIdsByCategoryIdForParser" resultMap="resultBean" >
				select
				ct.id article_id,ct.content_img litpic,c.*,ct.update_date as content_update_date
				FROM cms_content ct
				LEFT JOIN cms_category c ON ct.category_id = c.id
				where ct.del=0 and ct.content_display='0' and c.category_display='enable'
	
				<!-- 查询子栏目数据 -->
				<if test="categoryId!=null and  categoryId!='' and categoryType== '1'.toString()">
					and (ct.category_id=#{categoryId} or ct.category_id in
					(select id FROM cms_category where find_in_set(#{categoryId},CATEGORY_PARENT_IDS)>0))
				</if>
				<if test="categoryId!=null and  categoryId!='' and categoryType== '2'.toString()">
					and ct.category_id=#{categoryId}
				</if>
	
				<if test="endTime!=null and endTime!=''">
					<if test="_databaseId == 'mysql'">
						and ct.UPDATE_DATE &gt;= #{endTime}
					</if>
					<if test="_databaseId == 'oracle'">
						and ct.UPDATE_DATE &gt;= to_date(#{endTime}, 'yyyy-mm-dd hh24:mi:ss')
					</if>
				</if>
				<if test="flag!=null and flag!=''">
				and ct.content_type in ( #{flag})
				</if>
				<if  test="noflag!=null and noflag!=''">
				and (ct.content_type not in ( #{noflag}  ) or ct.content_type is null)
				</if>
				<if test="orderBy!=null  and orderBy!='' ">
					<if test="orderBy=='date'">ORDER BY content_datetime</if>
					<if test="orderBy=='hit'">ORDER BY content_hit</if>
					<if test="orderBy=='sort'">ORDER BY content_sort</if>
					<if  test="orderBy!='date' and orderBy!='hit' and orderBy!='sort'">
						ORDER BY ct.content_datetime
					</if>
					<choose>
						<when test="order!=null and order!=''">
							${order}
						</when>
						<otherwise>
							desc
						</otherwise>
					</choose>
				</if>
	
		</select>
	
	
		<!-- 根据站点编号、开始、结束时间和栏目编号查询文章编号集合,不包括单篇 -->
		<select id="queryIdsByCategoryIdForParserAndNotCover" resultMap="resultBean" >
			select
			ct.id article_id,ct.content_img  litpic,c.*
			FROM cms_content ct
			LEFT JOIN cms_category c ON ct.category_id = c.id
			where ct.del=0 and ct.content_display='0' and c.category_display='enable'
	
			<!-- 查询子栏目数据 -->
			<if test="categoryId!=null and  categoryId!='' and categoryType== '1'.toString()">
				and (ct.category_id=#{categoryId} or ct.category_id in
				(select id FROM cms_category where find_in_set(#{categoryId},CATEGORY_PARENT_IDS)>0 and category_type!='2'))
			</if>
			<if test="categoryId!=null and  categoryId!='' and categoryType== '2'.toString()">
				and ct.category_id=#{categoryId}
			</if>
			<if test="beginTime!=null and beginTime!=''">
				<if test="_databaseId == 'mysql'">
					AND ct.UPDATE_DATE &gt;=  #{beginTime}
				</if>
				<if test="_databaseId == 'oracle'">
					and ct.UPDATE_DATE &gt;= to_date(#{beginTime}, 'yyyy-mm-dd hh24:mi:ss')
				</if>
			</if>
			<if test="endTime!=null and endTime!=''">
				<if test="_databaseId == 'mysql'">
					and ct.UPDATE_DATE &gt;= #{endTime}
				</if>
				<if test="_databaseId == 'oracle'">
					and ct.UPDATE_DATE &gt;= to_date(#{endTime}, 'yyyy-mm-dd hh24:mi:ss')
				</if>
			</if>
			<if test="flag!=null and flag!=''">
				and ct.content_type in ( #{flag})
			</if>
			<if  test="noflag!=null and noflag!=''">
				and (ct.content_type not in ( #{noflag}  ) or ct.content_type is null)
			</if>
			<if test="orderBy!=null  and orderBy!='' ">
				<if test="orderBy=='date'">ORDER BY content_datetime</if>
				<if test="orderBy=='hit'">ORDER BY content_hit</if>
				<if test="orderBy=='sort'">ORDER BY content_sort</if>
				<if  test="orderBy!='date' and orderBy!='hit' and orderBy!='sort'">
					ORDER BY ct.content_datetime
				</if>
				<choose>
					<when test="order!=null and order!=''">
						${order}
					</when>
					<otherwise>
						desc
					</otherwise>
				</choose>
			</if>
	
		</select>
	
		<select id="getSearchCount" resultType="int">
			select count(*) from
			cms_content a
			left join cms_category c
			ON a.category_id
			= c.id
			<if test="tableName!=null and tableName!='' and diyList!=null">left join ${tableName} d on d.link_id=a.id
			</if>
			<where>
				a.del=0
				and a.content_display='0'
				and c.category_display='enable'
				and c.category_is_search='enable'
				<if test="categoryIds!=null and categoryIds!=''">
					and
					<foreach item="item" index="index" collection="categoryIds.split(',')" open="(" separator="or"
							 close=")">
						a.category_id=#{item}
						or a.category_id in (select id FROM cms_category where cms_category.del=0
						and FIND_IN_SET(#{item},CATEGORY_PARENT_IDS) > 0)
					</foreach>
				</if>
	
				<if test="map.content_title!=null">
				and a.content_title like CONCAT(CONCAT('%',#{map.content_title}),'%')
				</if>
				<if test="map.content_author!=null">
				and a.content_author like CONCAT(CONCAT('%',#{map.content_author}),'%')
				</if>
				<if test="map.content_source!=null">
				and a.content_source like CONCAT(CONCAT('%',#{map.content_source}),'%')
				</if>
				<if test="map.content_type!=null">
					and <foreach item="item" index="index" collection="map.content_type.split(',')"  open="(" separator="or" close=")">
						FIND_IN_SET(#{item},a.content_type)>0
					</foreach>
				</if>
				<if test="map.content_description!=null">
				and a.content_description like CONCAT(CONCAT('%',#{map.content_description}),'%')
				</if>
				<if test="map.content_tag!=null">
					and a.content_tags like CONCAT(CONCAT('%',#{map.content_tag}),'%')
				</if>
				<if test="map.content_keyword!=null">
				and a.content_keyword like CONCAT(CONCAT('%',#{map.content_keyword}),'%')
				</if>
				<if test="map.content_details!=null">
				and a.content_details like CONCAT(CONCAT('%',#{map.content_details}),'%')
				</if>
				<if test="map.content_datetime_start!=null and map.content_datetime_end!=null">
					<if test="_databaseId == 'mysql'">
						and a.content_datetime between #{map.content_datetime_start} and #{map.content_datetime_end}
					</if>
					<if test="_databaseId == 'oracle'">
						and a.content_datetime &gt; to_date(#{map.content_datetime_start}, 'yyyy-mm-dd hh24:mi:ss')
						and a.content_datetime &lt; to_date(#{map.content_datetime_end}, 'yyyy-mm-dd hh24:mi:ss')
					</if>
				</if>
				<if test="tableName!=null and tableName!='' and diyList!=null">
					<foreach item="item" index="index" collection="diyList" open=""
							 separator="" close="">
						and d.${item.key} like CONCAT(CONCAT('%',#{item.value}),'%')
					</foreach>
				</if>
			</where>
	
		</select>
	
	
	</mapper>
	`
	memEditor1 := memedit.NewMemEditor(code1)
	code2 := `package net.mingsoft.mdiy.action;

import cn.hutool.core.map.CaseInsensitiveMap;
import cn.hutool.core.util.ObjectUtil;
import com.baomidou.mybatisplus.core.conditions.Wrapper;
import com.baomidou.mybatisplus.core.conditions.query.LambdaQueryWrapper;
import io.swagger.annotations.Api;
import io.swagger.annotations.ApiImplicitParam;
import io.swagger.annotations.ApiImplicitParams;
import io.swagger.annotations.ApiOperation;
import java.util.Map;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import net.mingsoft.base.entity.ResultData;
import net.mingsoft.base.exception.BusinessException;
import net.mingsoft.basic.annotation.LogAnn;
import net.mingsoft.basic.constant.e.BusinessTypeEnum;
import net.mingsoft.basic.util.BasicUtil;
import net.mingsoft.mdiy.biz.IModelBiz;
import net.mingsoft.mdiy.biz.IModelDataBiz;
import net.mingsoft.mdiy.constant.e.ModelCustomTypeEnum;
import net.mingsoft.mdiy.entity.ModelEntity;
import org.apache.commons.lang3.StringUtils;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Controller;
import org.springframework.ui.ModelMap;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestMethod;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.ResponseBody;
import springfox.documentation.annotations.ApiIgnore;

@Api(
    tags = {"后端-自定义模块接口"}
)
@Controller
@RequestMapping({"/${ms.manager.path}/mdiy/form/data"})
public class FormDataAction extends BaseAction {
    @Autowired
    private IModelDataBiz modelDataBiz;
    @Autowired
    private IModelBiz modelBiz;

    public FormDataAction() {
    }

    @ApiIgnore
    @GetMapping({"/index"})
    public String index(HttpServletResponse response, HttpServletRequest request, @ApiIgnore ModelMap model) {
        String modelId = BasicUtil.getString("modelId");
        ModelEntity modelEntity = (ModelEntity)this.modelBiz.getOne((Wrapper)((LambdaQueryWrapper)(new LambdaQueryWrapper()).eq(ModelEntity::getId, modelId)).eq(ModelEntity::getModelCustomType, ModelCustomTypeEnum.FORM.getLabel()));
        if (modelEntity == null) {
            throw new BusinessException(this.getResString("err.not.exist", new String[]{this.getResString("model.id")}));
        } else {
            return "/mdiy/form/data/index";
        }
    }

    @ApiIgnore
    @GetMapping({"/form"})
    public String form(HttpServletResponse response, HttpServletRequest request, @ApiIgnore ModelMap model) {
        String businessForm = BasicUtil.getString("businessUrl");
        return StringUtils.isNotBlank(businessForm) ? businessForm : "/mdiy/form/data/form";
    }

    @ApiOperation("提供后台查询自定义表单提交数据")
    @ApiImplicitParams({@ApiImplicitParam(
    name = "modelId",
    value = "模型编号",
    required = true,
    paramType = "query"
), @ApiImplicitParam(
    name = "modelName",
    value = "模型名称",
    required = false,
    paramType = "query"
)})
    @RequestMapping(
        value = {"/queryData"},
        method = {RequestMethod.GET, RequestMethod.POST}
    )
    @ResponseBody
    public ResultData queryData(HttpServletRequest request, HttpServletResponse response) {
        Map<String, Object> map = BasicUtil.assemblyRequestMap();
        LambdaQueryWrapper<ModelEntity> wrapper = new LambdaQueryWrapper();
        ((LambdaQueryWrapper)wrapper.eq(ModelEntity::getId, map.get("modelId"))).eq(ModelEntity::getModelCustomType, ModelCustomTypeEnum.FORM.getLabel());
        ModelEntity modelEntity = (ModelEntity)this.modelBiz.getOne(wrapper, false);
        if (modelEntity == null) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("model.id")}));
        } else if (!this.hasPermissions("mdiy:formData:view", "mdiy:formData:" + modelEntity.getId() + ":view")) {
            return ResultData.build().error("没有权限!");
        } else {
            map.putIfAbsent("order", "desc");
            map.putIfAbsent("orderBy", "id");
            return ResultData.build().success(this.modelDataBiz.queryDiyFormData(modelEntity.getId(), map));
        }
    }

    @ApiOperation("提供后台查询自定义表单提交数据")
    @ApiImplicitParams({@ApiImplicitParam(
    name = "modelId",
    value = "模型编号",
    required = true,
    paramType = "query"
), @ApiImplicitParam(
    name = "id",
    value = "主键编号",
    required = true,
    paramType = "query"
)})
    @GetMapping({"/getData"})
    @ResponseBody
    public ResultData getData(HttpServletRequest request, HttpServletResponse response) {
        String modelId = BasicUtil.getString("modelId");
        String id = BasicUtil.getString("id");
        LambdaQueryWrapper<ModelEntity> wrapper = new LambdaQueryWrapper();
        wrapper.eq(ModelEntity::getId, modelId);
        wrapper.eq(ModelEntity::getModelCustomType, ModelCustomTypeEnum.FORM.getLabel());
        ModelEntity modelEntity = (ModelEntity)this.modelBiz.getOne(wrapper, false);
        if (modelEntity == null) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("model.id")}));
        } else if (StringUtils.isEmpty(id)) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("id")}));
        } else {
            Object object = this.modelDataBiz.getFormData(modelEntity.getId(), id);
            return ObjectUtil.isNotNull(object) ? ResultData.build().success(object) : ResultData.build().error();
        }
    }

    @ApiOperation("自定义业务数据保存")
    @ApiImplicitParams({@ApiImplicitParam(
    name = "modelName",
    value = "业务模型名称",
    required = true,
    paramType = "query"
), @ApiImplicitParam(
    name = "modelId",
    value = "业务模型Id",
    required = false,
    paramType = "query"
)})
    @LogAnn(
        title = "新增自定义业务数据",
        businessType = BusinessTypeEnum.INSERT
    )
    @PostMapping({"save"})
    @ResponseBody
    public ResultData save(HttpServletRequest request, HttpServletResponse response) {
        Map<String, Object> map = BasicUtil.assemblyRequestMap();
        CaseInsensitiveMap<String, Object> caseIgnoreMap = new CaseInsensitiveMap(map);
        String modelName = BasicUtil.getString("modelName");
        String modelId = BasicUtil.getString("modelId");
        if (StringUtils.isBlank(modelName) && StringUtils.isBlank(modelId)) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("model.id")})).code("mdiyErrCode");
        } else {
            LambdaQueryWrapper<ModelEntity> wrapper = new LambdaQueryWrapper();
            ((LambdaQueryWrapper)((LambdaQueryWrapper)wrapper.eq(StringUtils.isNotEmpty(modelName), ModelEntity::getModelName, modelName)).eq(StringUtils.isNotEmpty(modelId), ModelEntity::getId, modelId)).eq(ModelEntity::getModelCustomType, ModelCustomTypeEnum.FORM.getLabel());
            ModelEntity modelEntity = (ModelEntity)this.modelBiz.getOne(wrapper, true);
            if (modelEntity == null) {
                return ResultData.build().error(this.getResString("err.not.exist", new String[]{this.getResString("model.name")})).code("mdiyErrCode");
            } else if (!this.hasPermissions("mdiy:formData:save", "mdiy:formData:" + modelEntity.getId() + ":save")) {
                return ResultData.build().error("没有权限!").code("mdiyErrCode");
            } else {
                return this.modelDataBiz.saveDiyFormData(modelEntity.getId(), caseIgnoreMap) ? ResultData.build().success() : ResultData.build().error(this.getResString("err.error", new String[]{this.getResString("model.id")})).code("mdiyErrCode");
            }
        }
    }

    @ApiOperation("更新自定义业务数据")
    @ApiImplicitParam(
        name = "modelId",
        value = "模型编号",
        required = true,
        paramType = "query"
    )
    @LogAnn(
        title = "更新自定义业务数据",
        businessType = BusinessTypeEnum.UPDATE
    )
    @PostMapping({"update"})
    @ResponseBody
    public ResultData update(HttpServletRequest request, HttpServletResponse response) {
        Map<String, Object> map = BasicUtil.assemblyRequestMap();
        CaseInsensitiveMap<String, Object> caseIgnoreMap = new CaseInsensitiveMap(map);
        String modelId = caseIgnoreMap.get("modelId").toString();
        if (StringUtils.isBlank(modelId)) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("model.id")})).code("mdiyErrCode");
        } else {
            ModelEntity modelEntity = this.modelBiz.getById(modelId);
            if (!this.hasPermissions("mdiy:formData:update", "mdiy:formData:" + modelEntity.getId() + ":update")) {
                return ResultData.build().error("没有权限!").code("mdiyErrCode");
            } else {
                return this.modelDataBiz.updateDiyFormData(modelEntity, caseIgnoreMap) ? ResultData.build().success() : ResultData.build().error(this.getResString("err.error", new String[]{this.getResString("model.id")})).code("mdiyErrCode");
            }
        }
    }

    @ApiOperation("批量删除自定义业务数据接口")
    @LogAnn(
        title = "批量删除自定义业务数据接口",
        businessType = BusinessTypeEnum.DELETE
    )
    @PostMapping({"delete"})
    @ResponseBody
    public ResultData delete(@RequestParam("modelId") String modelId, HttpServletResponse response, HttpServletRequest request) {
        String ids = BasicUtil.getString("ids");
        if (StringUtils.isBlank(ids)) {
            return ResultData.build().error(this.getResString("err.error", new String[]{this.getResString("id")}));
        } else if (StringUtils.isBlank(modelId)) {
            return ResultData.build().error(this.getResString("err.empty", new String[]{this.getResString("model.id")}));
        } else {
            ModelEntity modelEntity = this.modelBiz.getById(modelId);
            if (!this.hasPermissions("mdiy:formData:del", "mdiy:formData:" + modelEntity.getId() + ":del")) {
                return ResultData.build().error("没有权限!");
            } else {
                String[] _ids = ids.split(",");
                String[] var7 = _ids;
                int var8 = _ids.length;

                for(int var9 = 0; var9 < var8; ++var9) {
                    String id = var7[var9];
                    this.modelDataBiz.deleteQueryDiyFormData(id, modelId);
                }

                return ResultData.build().success();
            }
        }
    }
}`
	memEditor2 := memedit.NewMemEditor(code2)
	fs.AddFile("/var/java/test1.xml", code1)
	fs.AddFile("/var/java/test1.java", code2)

	s := NewSfSearch(fs, t, ssaapi.WithLanguage(ssaconfig.JAVA))

	// search
	s.SearchAndCheck(t, "all", "$", true, map[string][]string{
		`"/${ms.manager.path}/mdiy/form/data"`: {memEditor2.GetTextFromPositionInt(39, 18, 39, 54)},
	})

	s.SearchAndCheck(t, "all", "${", false, map[string][]string{})

	s.SearchAndCheck(t, "all", "${", true, map[string][]string{
		`"/${ms.manager.path}/mdiy/form/data"`: {memEditor2.GetTextFromPositionInt(39, 18, 39, 54)},
		`"${order}"`:                           {memEditor1.GetTextFromPositionInt(359, 8, 359, 16), memEditor1.GetTextFromPositionInt(417, 7, 417, 15)},
		`"${tableName}"`:                       {memEditor1.GetTextFromPositionInt(433, 77, 433, 89)},
	})

	s.SearchAndCheck(t, "all", "${}", false, map[string][]string{})
	s.SearchAndCheck(t, "all", "${}", true, map[string][]string{
		`"/${ms.manager.path}/mdiy/form/data"`: {memEditor2.GetTextFromPositionInt(39, 18, 39, 54)},
		`"${order}"`:                           {memEditor1.GetTextFromPositionInt(359, 8, 359, 16), memEditor1.GetTextFromPositionInt(417, 7, 417, 15)},
		`"${tableName}"`:                       {memEditor1.GetTextFromPositionInt(433, 77, 433, 89)},
	})

	s.SearchAndCheck(t, "all", "${o}", false, map[string][]string{})
	s.SearchAndCheck(t, "all", "${order}", false, map[string][]string{
		`"${order}"`: {memEditor1.GetTextFromPositionInt(359, 8, 359, 16), memEditor1.GetTextFromPositionInt(417, 7, 417, 15)},
	})
	s.SearchAndCheck(t, "all", "${o}", true, map[string][]string{
		`"${order}"`: {memEditor1.GetTextFromPositionInt(359, 8, 359, 16), memEditor1.GetTextFromPositionInt(417, 7, 417, 15)},
	})
}
