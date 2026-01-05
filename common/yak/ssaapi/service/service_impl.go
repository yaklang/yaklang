package service

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// ssaServiceImpl 实现 SSAService 接口
type ssaServiceImpl struct{}

// NewSSAService 创建新的 SSA 服务实例
func NewSSAService() SSAService {
	return &ssaServiceImpl{}
}

// Compile 编译项目
func (s *ssaServiceImpl) Compile(ctx context.Context, req *SSACompileRequest) (*SSACompileResponse, error) {
	if req == nil {
		return &SSACompileResponse{
			Error: utils.Error("compile request is nil"),
		}, utils.Error("compile request is nil")
	}

	// Target 或 Options 中的 FileSystem 必须提供一个
	// 这个检查会在后面更精确地进行

	// 构建编译选项
	opt := make([]ssaconfig.Option, 0, 5)

	// 添加语言选项
	if req.Language != "" {
		opt = append(opt, ssaapi.WithRawLanguage(req.Language))
	}

	// 添加重新编译选项
	opt = append(opt, ssaapi.WithReCompile(req.ReCompile))

	// 添加排除文件选项
	if req.ExcludeFile != "" {
		opt = append(opt, ssaapi.WithExcludeFunc(req.ExcludeFile))
	}

	// 添加入口文件选项
	if req.Entry != "" {
		log.Infof("start to use entry file: %v", req.Entry)
		opt = append(opt, ssaapi.WithFileSystemEntry(req.Entry))
	}

	// 添加程序名称选项
	if req.ProgramName != "" {
		log.Infof("compile save to database with program name: %v", req.ProgramName)
		opt = append(opt, ssaapi.WithProgramName(req.ProgramName))
	}

	// 合并用户自定义选项
	opt = append(opt, req.Options...)

	var proj ssaapi.Programs
	var err error

	// 如果 Target 为空，尝试直接使用 ParseProject（假设 Options 中已包含 FileSystem）
	if req.Target == "" {
		_, tempErr := ssaapi.DefaultConfig(opt...)
		if tempErr != nil {
			errMsg := tempErr.Error()
			if strings.Contains(errMsg, "file system") || strings.Contains(errMsg, "origin editor") {
				return &SSACompileResponse{
					Error: utils.Error("target file or directory is required"),
				}, utils.Error("target file or directory is required")
			}
			return &SSACompileResponse{
				Error: utils.Errorf("parse project failed: %v", tempErr),
			}, tempErr
		}
		proj, err = ssaapi.ParseProject(opt...)
		if err != nil {
			return &SSACompileResponse{
				Error: utils.Errorf("parse project failed: %v", err),
			}, err
		}
	} else {
		// 尝试作为 zip 文件处理
		zipfs, err := filesys.NewZipFSFromLocal(req.Target)
		if err == nil {
			// 是 zip 文件
			proj, err = ssaapi.ParseProjectWithFS(zipfs, opt...)
			if err != nil {
				return &SSACompileResponse{
					Error: utils.Errorf("parse project [%v] failed: %v", req.Target, err),
				}, err
			}
		} else {
			// 是普通目录或文件
			proj, err = ssaapi.ParseProjectFromPath(req.Target, opt...)
			if err != nil {
				return &SSACompileResponse{
					Error: utils.Errorf("parse project [%v] failed: %v", req.Target, err),
				}, err
			}
		}
	}

	log.Infof("finished compiling..., results: %v", len(proj))
	return &SSACompileResponse{
		Programs: proj,
		Error:    nil,
	}, nil
}

// QueryPrograms 查询程序
func (s *ssaServiceImpl) QueryPrograms(ctx context.Context, req *SSAQueryRequest) (*SSAQueryResponse, error) {
	if req == nil {
		return &SSAQueryResponse{
			Error: utils.Error("query request is nil"),
		}, utils.Error("query request is nil")
	}

	// 默认匹配所有
	pattern := req.ProgramNamePattern
	if pattern == "" {
		pattern = ".*"
	}

	// 查询程序
	programs := ssaapi.LoadProgramRegexp(pattern)

	// 应用语言过滤
	if req.Language != "" {
		filtered := make([]*ssaapi.Program, 0)
		for _, p := range programs {
			if string(p.GetLanguage()) == req.Language {
				filtered = append(filtered, p)
			}
		}
		programs = filtered
	}

	// 应用数量限制
	if req.Limit > 0 && len(programs) > req.Limit {
		programs = programs[:req.Limit]
	}

	return &SSAQueryResponse{
		Programs: programs,
		Error:    nil,
	}, nil
}

// SyntaxFlowQuery 执行SyntaxFlow查询
func (s *ssaServiceImpl) SyntaxFlowQuery(ctx context.Context, req *SSASyntaxFlowQueryRequest) (*SSASyntaxFlowQueryResponse, error) {
	if req == nil {
		return &SSASyntaxFlowQueryResponse{
			Error: utils.Error("syntaxflow query request is nil"),
		}, utils.Error("syntaxflow query request is nil")
	}

	// 检查程序名称是否为空
	if req.ProgramName == "" {
		return &SSASyntaxFlowQueryResponse{
			Error: utils.Error("program name is required when using syntax flow query language"),
		}, utils.Error("program name is required when using syntax flow query language")
	}

	// 从数据库加载程序
	prog, err := ssaapi.FromDatabase(req.ProgramName)
	if err != nil {
		return &SSASyntaxFlowQueryResponse{
			Error: utils.Errorf("load program [%v] from database failed: %v", req.ProgramName, err),
		}, err
	}

	// 构建查询选项
	opt := make([]ssaapi.QueryOption, 0)
	if req.Debug {
		opt = append(opt, ssaapi.QueryWithEnableDebug())
	}

	// 执行SyntaxFlow查询
	result, err := prog.SyntaxFlowWithError(req.Rule, opt...)
	if err != nil {
		var otherErrs []string
		if result != nil && len(result.GetErrors()) > 0 {
			otherErrs = utils.StringArrayFilterEmpty(utils.RemoveRepeatStringSlice(result.GetErrors()))
		}
		err = utils.Wrapf(err, "prompt error: \n%v", strings.Join(otherErrs, "\n  "))
	}

	if result == nil {
		return &SSASyntaxFlowQueryResponse{
			Error: err,
		}, err
	}

	return &SSASyntaxFlowQueryResponse{
		Result: result,
		Error:  err,
	}, err
}

