package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/shlex"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func status(verbose string, desc string) string {
	raw, err := json.Marshal(map[string]string{
		"status":  verbose,
		"message": desc,
	})
	if err != nil {
		return ""
	}
	return string(raw)
}

func (s *Server) execRequest(req *ypb.ExecRequest, moduleName string, ctx context.Context, handler func(
	result *ypb.ExecResult, logInfo *yaklib.YakitLog,
) error, writer io.Writer) error {
	runtimeId := uuid.NewV4().String()

	log.Infof("start to call exec-yak: %p", req)
	yakEnginePath, err := os.Executable()
	if err != nil {
		return utils.Errorf("cannot found yak engine binary...: %s", err)
	}

	//var messages = &[]string{}
	yakitServer := yaklib.NewYakitServer(0,
		yaklib.SetYakitServer_ProgressHandler(func(id string, progress float64) {
			raw, _ := yaklib.YakitMessageGenerator(&yaklib.YakitProgress{
				Id:       id,
				Progress: progress,
			})
			if raw != nil {
				err = handler(&ypb.ExecResult{
					Hash:       "",
					OutputJson: "",
					Raw:        nil,
					IsMessage:  true,
					Message:    raw,
					Id:         0,
					RuntimeID:  runtimeId,
				}, nil)
				if err != nil {
					log.Errorf("send execResult message error: %v", err)
					//} else {
					//	*messages = append(*messages, string(raw))
				}
			}
		}),
		yaklib.SetYakitServer_LogHandler(func(level string, info string) {
			logItem := &yaklib.YakitLog{
				Level:     level,
				Data:      info,
				Timestamp: time.Now().Unix(),
			}
			SaveFromYakitLog(logItem, s.GetProjectDatabase())

			raw, _ := yaklib.YakitMessageGenerator(logItem)
			if raw != nil {
				err = handler(&ypb.ExecResult{
					IsMessage: true,
					Message:   raw,
				}, logItem)
				if err != nil {
					log.Errorf("send execResult message error: %v", err)
				}
			}
		}),
	)
	yakitServer.Start()
	defer yakitServer.Shutdown()

	log.Info("start to handing params")
	var params = []string{
		"--yakit-webhook", yakitServer.Addr(),
	}
	for _, p := range req.GetParams() {
		switch p.Key {
		case "__yakit_plugin_names__":
			var fp, err = os.CreateTemp(os.TempDir(), "yakit-plugin-selector-*.txt")
			if err != nil {
				return utils.Errorf("create yakit plugin selector failed: %s", err)
			}
			fp.WriteString(p.Value)
			fp.Close()
			params = append(params, "--yakit-plugin-file", fp.Name())
			continue
		}

		if strings.HasPrefix(p.Key, "-") {
			params = append(params, p.Key, p.Value)
		} else {
			if p.Value == "" {
				params = append(params, fmt.Sprintf("--%v", p.Key))
			} else {
				params = append(params, fmt.Sprintf("--%v", p.Key), p.Value)
			}
		}
	}

	if req.GetRunnerParamRaw() != "" {
		var paramsExisted = strings.Join(params, " ")
		paramsRaw := fmt.Sprintf("%v %v", paramsExisted, req.GetRunnerParamRaw())
		finalParams, err := shlex.Split(paramsRaw)
		if err != nil {
			log.Errorf("shlex parse %v failed: %s", paramsRaw, err)
		} else {
			params = finalParams
		}
	}

	log.Info("start to fetch/handling yak code...")
	var code = req.GetScript()
	var scriptId = req.GetScriptId()
	if code == "" {
		if scriptId == "" {
			return utils.Errorf("fetch yak code failed: %s", "empty code and scriptId")
		}
		script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), scriptId)
		if err != nil {
			return utils.Errorf("cannot find script yak code by scriptId:[%s]", scriptId)
		}
		code = script.Content
	}
	f, err := ioutil.TempFile("", "yaki-code-*.yak")
	if err != nil {
		return utils.Errorf("create temp file(for saving yak code) failed: %s", err)
	}
	_, err = f.WriteString(code)
	if err != nil {
		return utils.Errorf("save yak code to %v failed: %s", f.Name(), err)
	}
	f.Close()
	defer os.RemoveAll(f.Name())

	utils.Debug(func() {
		raw, err := ioutil.ReadFile(f.Name())
		if err != nil {
			log.Errorf("verify %v failed: %s", f.Name(), err)
			return
		}
		fmt.Println(string(raw))
	})
	cmd := exec.CommandContext(
		ctx, yakEnginePath,
		append([]string{f.Name()}, params...)...)

	cmd.Env = append(cmd.Env, os.Environ()...) // 继承主进程环境变量？

	// 配置 YAKIT_HOME 环境变量
	cmd.Env = append(cmd.Env, fmt.Sprintf("YAKIT_HOME=%v", os.Getenv("YAKIT_HOME")))

	// 配置运行时变量名
	cmd.Env = append(cmd.Env, fmt.Sprintf("YAK_RUNTIME_ID=%v", runtimeId))

	// 运行时 ID
	cmd.Env = append(cmd.Env, fmt.Sprintf("YAKIT_PLUGIN_ID=%v", moduleName))

	// 配置默认数据库名
	for k, v := range map[string]string{
		consts.CONST_YAK_DEFAULT_PROFILE_DATABASE_NAME: consts.YAK_PROFILE_PLUGIN_DB_NAME,
		consts.CONST_YAK_DEFAULT_PROJECT_DATABASE_NAME: consts.YAK_PROJECT_DATA_DB_NAME,
	} {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%v=%v", k, v))
	}

	// 添加环境变量，远程 YAK_BRIDGE_REMOTE_REVERSE_ADDR
	if remoteReverseIP != "" && remoteReversePort > 0 {
		cmd.Env = append(
			cmd.Env,
			fmt.Sprintf("YAK_BRIDGE_REMOTE_REVERSE_ADDR=%v", utils.HostPort(remoteReverseIP, remoteReversePort)),
			fmt.Sprintf("YAK_BRIDGE_ADDR=%v", remoteAddr),
			fmt.Sprintf("YAK_BRIDGE_SECRET=%v", remoteSecret),
			GetScanProxyEnviron(),
		)
	}
	// 添加环境变量 本地 YAK_BRIDGE_REMOTE_REVERSE_ADDR
	if localReverseHost != "" {
		cmd.Env = append(
			cmd.Env,
			fmt.Sprintf("YAK_BRIDGE_LOCAL_REVERSE_ADDR=%v", utils.HostPort(localReverseHost, s.reverseServer.Port)),
		)
	}

	// 添加
	log.Infof("start to exec params: %v binary: %v", params, yakEnginePath)

	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer

	cmd.Stdout = io.MultiWriter(writer, &stdoutBuffer, os.Stdout)
	cmd.Stderr = io.MultiWriter(writer, &stderrBuffer, os.Stderr)

	start := time.Now()
	err = cmd.Run()
	history := &yakit.ExecHistory{
		Script:        code,
		RuntimeId:     runtimeId,
		ScriptId:      scriptId,
		FromYakModule: moduleName,
		TimestampNano: start.UnixNano(),
		DurationMs:    time.Now().Sub(start).Milliseconds(),
		Params:        strings.Join(params, " "),
		Stdout:        strconv.Quote(stdoutBuffer.String()),
		Stderr:        strconv.Quote(stderrBuffer.String()),
	}
	//defer func() {
	//	raw, _ := json.Marshal(*messages)
	//	if raw != nil {
	//		history.Messages = strconv.Quote(string(raw))
	//	}
	//	history.RuntimeId = runtimeId
	//	err := yakit.CreateOrUpdateExecHistory(s.db, history.CalcHash(), history)
	//	if err != nil {
	//		log.Errorf("save exec history failed: %s", err)
	//		return
	//	}
	//}()

	if err != nil {
		history.Ok = false
		history.Reason = fmt.Sprintf("execute yak code failed: %s", err)
		log.Errorf("execute yak code failed: %s", err)
		return err
	}

	history.Ok = true
	return nil
}

func (s *Server) Exec(req *ypb.ExecRequest, stream ypb.Yak_ExecServer) error {
	return s.ExecWithContext(stream.Context(), req, stream)
}

func (s *Server) ExecWithContext(ctx context.Context, req *ypb.ExecRequest, stream ypb.Yak_ExecServer) error {
	if ctx == nil {
		ctx = stream.Context()
	}
	return s.execRequest(req, req.ScriptId, ctx, func(result *ypb.ExecResult, _ *yaklib.YakitLog) error {
		return stream.Send(result)
	}, &YakOutputStreamerHelperWC{
		stream: stream,
	})
}

func (s *Server) YaklangCompileAndFormat(cx context.Context, req *ypb.YaklangCompileAndFormatRequest) (*ypb.YaklangCompileAndFormatResponse, error) {
	newCode, err := antlr4yak.New().FormattedAndSyntaxChecking(req.GetCode())
	if err != nil {
		return nil, err
	}
	return &ypb.YaklangCompileAndFormatResponse{Code: newCode}, nil
}
