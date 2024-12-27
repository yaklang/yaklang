package scan_process

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/kafka"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"os"
	"os/exec"
	"strings"
	"time"
)

type ScriptProcessor struct {
	hookServer *yaklib.YakitServer
	config     *kafka.TaskConfig
	//script cache
	cache *utils.Cache[string]
}

func (s *ScriptProcessor) Process(ctx context.Context, message *kafka.TaskRequestMessage) {
	var tmpMap map[string]any
	err := json.Unmarshal(message.Params, &tmpMap)
	if err != nil {
		s.config.OnTaskErrorFunc(message.TaskId, err)
		return
	}
	scanNodePath, err := os.Executable()
	if err != nil {
		s.config.OnTaskErrorFunc(message.TaskId, utils.Errorf("rpc call InvokeScript failed: fetch node path err: %s", err))
		return
	}
	var params = []string{"--yakit-webhook", s.hookServer.Addr()}
	for k, v := range tmpMap {
		k = strings.TrimLeft(k, "-")
		params = append(params, "--"+k)
		params = append(params, utils.InterfaceToString(v))
	}
	var fname string
	_filename, exists := s.cache.Get(message.TaskId)
	if !exists {
		filename := uuid.NewString()
		f, err := consts.TempFile(fmt.Sprintf("%s.yak", filename))
		if err != nil {
			s.config.OnTaskErrorFunc(message.TaskId, err)
			return
		}
		_, err2 := f.Write(message.Content)
		if err2 != nil {
			s.config.OnTaskErrorFunc(message.TaskId, err2)
			return
		}
		_ = f.Close()
		s.cache.Set(message.TaskId, filename)
		fname = filename
	} else {
		fname = _filename
	}
	baseCmd := []string{"distyak", fname}
	log.Infof("yak %v %v", fname, params)
	cmd := exec.CommandContext(ctx, scanNodePath, append(baseCmd, params...)...)
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("YAKIT_HOME=%v", os.Getenv("YAKIT_HOME")))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
}

func (s *ScriptProcessor) Finish() {
	s.hookServer.Shutdown()
}

func (s *ScriptProcessor) SetContext(config *kafka.TaskConfig) {
	s.config = config
}

func (s *ScriptProcessor) Type() kafka.TaskType {
	return kafka.Script
}

func (s *ScriptProcessor) Init(ctx context.Context, config *kafka.TaskConfig) {
	s.cache = utils.NewTTLCache[string](time.Second * time.Duration(60*15))
	s.cache.SetExpirationCallback(func(key string, value string) {
		_ = os.Remove(value)
	})
	server := yaklib.NewYakitServer(0,
		yaklib.SetYakitServer_LogHandler(func(level string, info string) {
			/*
				只处理json类型的数据，yakit.output时，需维护类型
				{
				"taskId":
				"result": "json",
				}
			*/
			var tmpMap map[string]any
			switch level {
			case "json":
				err := json.Unmarshal(codec.AnyToBytes(info), &tmpMap)
				if err != nil {
					log.Debugf("process json fail: %s", err)
					return
				}
				id, exist := tmpMap["taskId"]
				result, exist2 := tmpMap["result"]
				if !exist || !exist2 {
					log.Debug("process this output fail")
					return
				}
				config.OnTaskResultBackFunc("", codec.AnyToString(id), result)
			default:
			}
		}),
	)
	server.Start()
}
func init() {
	kafka.RegisterProcess(&ScriptProcessor{})
}
