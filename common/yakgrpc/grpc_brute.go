package yakgrpc

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed grpc_brute.yak
var startBruteScript string

func (s *Server) StartBrute(params *ypb.StartBruteParams, stream ypb.Yak_StartBruteServer) error {
	execParams := make([]*ypb.KVPair, 0)
	// reqParams := &ypb.ExecRequest{Script: startBruteScript}

	types := utils.PrettifyListFromStringSplited(params.GetType(), ",")
	for _, t := range types {
		h, err := bruteutils.GetBruteFuncByType(t)
		if err != nil || h == nil {
			return utils.Errorf("brute type: %v is not available", t)
		}
	}
	execParams = append(execParams, &ypb.KVPair{
		Key:   "types",
		Value: params.GetType(),
	})

	targetFile, err := utils.DumpHostFileWithTextAndFiles(params.Targets, "\n", params.TargetFile)
	if err != nil {
		return err
	}
	defer os.RemoveAll(targetFile)
	execParams = append(execParams, &ypb.KVPair{
		Key:   "target-file",
		Value: targetFile,
	})

	// 解析用户名
	userListFile, err := utils.DumpFileWithTextAndFiles(
		strings.Join(params.Usernames, "\n"), "\n", params.UsernameFile,
	)
	if err != nil {
		return err
	}
	defer os.RemoveAll(userListFile)
	execParams = append(execParams, &ypb.KVPair{
		Key:   "user-list-file",
		Value: userListFile,
	})

	// 是否使用默认字典？
	if params.GetReplaceDefaultPasswordDict() {
		execParams = append(execParams, &ypb.KVPair{
			Key:   "replace-default-password-dict",
			Value: "",
		})
	}

	if params.GetReplaceDefaultUsernameDict() {
		execParams = append(execParams, &ypb.KVPair{
			Key:   "replace-default-username-dict",
			Value: "",
		})
	}

	// 解析密码
	passListFile, err := utils.DumpFileWithTextAndFiles(
		strings.Join(params.Passwords, "\n"), "\n", params.PasswordFile,
	)
	if err != nil {
		return err
	}
	defer os.RemoveAll(passListFile)
	execParams = append(execParams, &ypb.KVPair{
		Key:   "pass-list-file",
		Value: passListFile,
	})

	// ok to stop
	if params.GetOkToStop() {
		execParams = append(execParams, &ypb.KVPair{Key: "ok-to-stop", Value: ""})
	}

	if params.GetConcurrent() > 0 {
		execParams = append(execParams, &ypb.KVPair{Key: "concurrent", Value: fmt.Sprint(params.GetConcurrent())})
	}

	if params.GetTargetTaskConcurrent() > 0 {
		execParams = append(execParams, &ypb.KVPair{Key: "task-concurrent", Value: fmt.Sprint(params.GetTargetTaskConcurrent())})
	}

	if params.GetDelayMin() > 0 && params.GetDelayMax() > 0 {
		execParams = append(execParams, &ypb.KVPair{Key: "delay-min", Value: fmt.Sprint(params.GetDelayMin())})
		execParams = append(execParams, &ypb.KVPair{Key: "delay-max", Value: fmt.Sprint(params.GetDelayMax())})
	}

	return s.debugScript(
		"", "yak", startBruteScript, stream, execParams, nil,
	)
}

func (s *Server) GetAvailableBruteTypes(ctx context.Context, req *ypb.Empty) (*ypb.GetAvailableBruteTypesResponse, error) {
	return &ypb.GetAvailableBruteTypesResponse{Types: bruteutils.GetBuildinAvailableBruteType()}, nil
}
