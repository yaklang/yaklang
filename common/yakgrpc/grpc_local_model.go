package yakgrpc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// LocalModelManager 本地模型管理器
type LocalModelManager struct {
	mutex         sync.RWMutex
	runningModels map[string]*exec.Cmd
}

var localModelManager = &LocalModelManager{
	runningModels: make(map[string]*exec.Cmd),
}

// getSupportedModels 获取支持的模型列表
func getSupportedModels() []*ypb.LocalModelConfig {
	return []*ypb.LocalModelConfig{
		{
			Name:        "Qwen3-Embedding-0.6B-Q8_0",
			Type:        "embedding",
			FileName:    "Qwen3-Embedding-0.6B-Q8_0.gguf",
			DownloadURL: "https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/main/Qwen3-Embedding-0.6B-Q8_0.gguf?download=true",
			Description: "Qwen3 Embedding 0.6B Q8_0 - 文本嵌入模型",
			DefaultPort: 8080,
		},
	}
}

// GetSupportedLocalModels 获取支持的本地模型列表
func (s *Server) GetSupportedLocalModels(ctx context.Context, req *ypb.Empty) (*ypb.GetSupportedLocalModelsResponse, error) {
	models := getSupportedModels()
	return &ypb.GetSupportedLocalModelsResponse{
		Models: models,
	}, nil
}

// IsLlamaServerReady 检查llama-server是否已安装并可用
func (s *Server) IsLlamaServerReady(ctx context.Context, req *ypb.Empty) (*ypb.IsLlamaServerReadyResponse, error) {
	llamaServerPath := consts.GetLlamaServerPath()
	if llamaServerPath == "" {
		return &ypb.IsLlamaServerReadyResponse{
			Ok:     false,
			Reason: "llama-server 未安装",
		}, nil
	}

	return &ypb.IsLlamaServerReadyResponse{
		Ok: true,
	}, nil
}

// IsLocalModelReady 检查本地模型是否就绪
func (s *Server) IsLocalModelReady(ctx context.Context, req *ypb.IsLocalModelReadyRequest) (*ypb.IsLocalModelReadyResponse, error) {
	modelName := req.GetModelName()
	if modelName == "" {
		return &ypb.IsLocalModelReadyResponse{
			Ok:     false,
			Reason: "模型名称不能为空",
		}, nil
	}

	// 检查llama-server是否存在
	llamaServerPath := consts.GetLlamaServerPath()
	if llamaServerPath == "" {
		return &ypb.IsLocalModelReadyResponse{
			Ok:     false,
			Reason: "llama-server 未安装",
		}, nil
	}

	// 查找模型配置
	var targetModel *ypb.LocalModelConfig
	models := getSupportedModels()
	for _, model := range models {
		if model.Name == modelName {
			targetModel = model
			break
		}
	}

	if targetModel == nil {
		return &ypb.IsLocalModelReadyResponse{
			Ok:     false,
			Reason: fmt.Sprintf("不支持的模型: %s", modelName),
		}, nil
	}

	// 检查模型文件是否存在
	modelsDir := consts.GetAIModelPath()
	if modelsDir == "" {
		return &ypb.IsLocalModelReadyResponse{
			Ok:     false,
			Reason: "无法获取AI模型存储路径",
		}, nil
	}

	modelPath := filepath.Join(modelsDir, targetModel.FileName)
	if exists, _ := utils.PathExists(modelPath); !exists {
		return &ypb.IsLocalModelReadyResponse{
			Ok:     false,
			Reason: fmt.Sprintf("模型文件不存在: %s", modelPath),
		}, nil
	}

	return &ypb.IsLocalModelReadyResponse{
		Ok: true,
	}, nil
}

// InstallLocalModel 安装本地模型（主要是下载llama-server）
func (s *Server) InstallLlamaServer(req *ypb.InstallLlamaServerRequest, stream ypb.Yak_InstallLlamaServerServer) error {
	tempFileName := "llama-b5702.zip"
	err := s.DownloadWithStream(req.GetProxy(), func() (urlStr string, name string, err error) {
		if utils.IsWindows() {
			return "https://github.com/ggml-org/llama.cpp/releases/download/b5702/llama-b5702-bin-win-cpu-x64.zip", tempFileName, nil
		}

		if utils.IsLinux() {
			return "https://github.com/ggml-org/llama.cpp/releases/download/b5702/llama-b5702-bin-ubuntu-x64.zip", tempFileName, nil
		}

		if utils.IsMac() {
			if runtime.GOARCH == "arm64" {
				return "https://github.com/ggml-org/llama.cpp/releases/download/b5712/llama-b5712-bin-macos-arm64.zip", tempFileName, nil
			} else {
				return "https://github.com/ggml-org/llama.cpp/releases/download/b5702/llama-b5702-bin-macos-x64.zip", tempFileName, nil
			}
		}
		return "", "", utils.Error("unsupported os: " + runtime.GOOS)
	}, stream)

	if err != nil {
		return err
	}

	dirPath := filepath.Join(
		consts.GetDefaultYakitProjectsDir(),
		"libs",
	)

	tempFilePath := filepath.Join(dirPath, tempFileName)

	exists, err := utils.PathExists(tempFilePath)
	if err != nil || !exists {
		return utils.Errorf("下载失败: %v", err)
	}

	// 解压文件到目标目录
	stream.Send(&ypb.ExecResult{
		IsMessage: true,
		Message:   []byte("正在解压文件..."),
	})

	targetPath := filepath.Join(dirPath, "llama-server")
	os.MkdirAll(targetPath, 0755)
	err = ziputil.DeCompress(tempFilePath, targetPath)
	if err != nil {
		return utils.Errorf("解压文件失败: %v", err)
	}

	stream.Send(&ypb.ExecResult{
		IsMessage: true,
		Message:   []byte("解压完成"),
	})

	os.Remove(tempFilePath)
	// 验证安装是否成功
	llamaServerPath := consts.GetLlamaServerPath()
	if llamaServerPath == "" {
		return utils.Errorf("下载完成，但llama-server不可用")
	}

	// chmod +x llama-server
	os.Chmod(llamaServerPath, 0755)

	return nil
}

// DownloadLocalModel 下载本地模型
func (s *Server) DownloadLocalModel(req *ypb.DownloadLocalModelRequest, stream ypb.Yak_DownloadLocalModelServer) error {
	modelName := req.GetModelName()
	if modelName == "" {
		return utils.Error("模型名称不能为空")
	}

	// 查找模型配置
	var targetModel *ypb.LocalModelConfig
	models := getSupportedModels()
	for _, model := range models {
		if model.Name == modelName {
			targetModel = model
			break
		}
	}

	if targetModel == nil {
		return utils.Errorf("不支持的模型: %s", modelName)
	}

	modelsDir := consts.GetAIModelPath()
	if modelsDir == "" {
		return utils.Error("无法获取AI模型存储路径")
	}

	stream.Send(&ypb.ExecResult{
		IsMessage: true,
		Message:   []byte(fmt.Sprintf("开始下载模型: %s", modelName)),
	})

	// 使用DownloadWithStream下载模型
	err := s.DownloadWithStream(req.GetProxy(), func() (urlStr string, name string, err error) {
		return targetModel.DownloadURL, filepath.Join("models", targetModel.FileName), nil
	}, stream)

	if err != nil {
		return utils.Errorf("下载模型失败: %v", err)
	}

	stream.Send(&ypb.ExecResult{
		IsMessage: true,
		Message:   []byte(fmt.Sprintf("模型 %s 下载完成", modelName)),
	})

	return nil
}

// StartLocalModel 启动本地模型
func (s *Server) StartLocalModel(req *ypb.StartLocalModelRequest, stream ypb.Yak_StartLocalModelServer) error {
	modelName := req.GetModelName()
	if modelName == "" {
		return utils.Error("模型名称不能为空")
	}

	// 检查模型是否已在运行
	localModelManager.mutex.RLock()
	if cmd, exists := localModelManager.runningModels[modelName]; exists && cmd.ProcessState == nil {
		localModelManager.mutex.RUnlock()
		return utils.Errorf("模型 %s 已在运行中", modelName)
	}
	localModelManager.mutex.RUnlock()

	// 检查模型是否就绪
	readyResp, err := s.IsLocalModelReady(stream.Context(), &ypb.IsLocalModelReadyRequest{ModelName: modelName})
	if err != nil {
		return err
	}

	if !readyResp.Ok {
		return utils.Errorf("模型未就绪: %s", readyResp.Reason)
	}

	// 查找模型配置
	var targetModel *ypb.LocalModelConfig
	models := getSupportedModels()
	for _, model := range models {
		if model.Name == modelName {
			targetModel = model
			break
		}
	}

	if targetModel == nil {
		return utils.Errorf("不支持的模型: %s", modelName)
	}

	// 构建启动命令
	llamaServerPath := consts.GetLlamaServerPath()
	modelsDir := consts.GetAIModelPath()
	modelPath := filepath.Join(modelsDir, targetModel.FileName)

	host := req.GetHost()
	if host == "" {
		host = "127.0.0.1"
	}

	port := req.GetPort()
	if port == 0 {
		port = targetModel.DefaultPort
	}

	args := []string{
		"-m", modelPath,
		"--host", host,
		"--port", fmt.Sprintf("%d", port),
		"--verbose-prompt",
	}

	if targetModel.Type == "embedding" {
		args = append(args, "--embedding")
	}

	stream.Send(&ypb.ExecResult{
		IsMessage: true,
		Message:   []byte(fmt.Sprintf("启动模型: %s，端口: %d", modelName, port)),
	})

	cmd := exec.CommandContext(stream.Context(), llamaServerPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Infof("启动命令: %s", cmd.String())
	err = cmd.Start()
	if err != nil {
		return utils.Errorf("启动模型失败: %v", err)
	}

	// 保存运行中的模型
	localModelManager.mutex.Lock()
	localModelManager.runningModels[modelName] = cmd
	localModelManager.mutex.Unlock()

	stream.Send(&ypb.ExecResult{
		IsMessage: true,
		Message:   []byte(fmt.Sprintf("模型 %s 启动成功，PID: %d", modelName, cmd.Process.Pid)),
	})

	// 等待一段时间以确保服务启动
	time.Sleep(3 * time.Second)

	// 监听上下文取消和进程结束
	ctx := stream.Context()
	done := make(chan error, 1)

	// 在单独的goroutine中等待进程结束
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// 用户取消了操作，结束进程
		log.Infof("检测到上下文取消，停止模型 %s，PID: %d", modelName, cmd.Process.Pid)
		if cmd.Process != nil {
			err := cmd.Process.Kill()
			if err != nil {
				log.Warnf("强制停止模型进程失败: %v", err)
			}
		}

		// 等待进程完全结束
		<-done

		// 清理运行中的模型记录
		localModelManager.mutex.Lock()
		delete(localModelManager.runningModels, modelName)
		localModelManager.mutex.Unlock()

		stream.Send(&ypb.ExecResult{
			IsMessage: true,
			Message:   []byte(fmt.Sprintf("模型 %s 已被用户取消并停止", modelName)),
		})

		return ctx.Err()

	case err := <-done:
		// 进程正常结束
		localModelManager.mutex.Lock()
		delete(localModelManager.runningModels, modelName)
		localModelManager.mutex.Unlock()

		if err != nil {
			log.Errorf("模型 %s 进程异常结束: %v", modelName, err)
			stream.Send(&ypb.ExecResult{
				IsMessage: true,
				Message:   []byte(fmt.Sprintf("模型 %s 进程异常结束: %v", modelName, err)),
			})
			return utils.Errorf("模型进程异常结束: %v", err)
		} else {
			log.Infof("模型 %s 进程正常结束", modelName)
			stream.Send(&ypb.ExecResult{
				IsMessage: true,
				Message:   []byte(fmt.Sprintf("模型 %s 进程已结束", modelName)),
			})
			return nil
		}
	}
}
