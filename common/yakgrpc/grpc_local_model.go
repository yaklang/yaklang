package yakgrpc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
			Name:        "Qwen3-Embedding-0.6B-Q4_K_M",
			Type:        "embedding",
			FileName:    "Qwen3-Embedding-0.6B-Q4_K_M.gguf",
			DownloadURL: "https://oss-qn.yaklang.com/gguf/Qwen3-Embedding-0.6B-Q4_K_M.gguf",
			Description: "Qwen3 Embedding 0.6B Q4_K_M - 文本嵌入模型",
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
	modelPath := consts.GetAIModelFilePath(targetModel.FileName)
	if modelPath == "" {
		return &ypb.IsLocalModelReadyResponse{
			Ok:     false,
			Reason: fmt.Sprintf("模型文件不存在: %s", targetModel.FileName),
		}, nil
	}

	return &ypb.IsLocalModelReadyResponse{
		Ok: true,
	}, nil
}

// InstallLocalModel 安装本地模型（主要是下载llama-server）
func (s *Server) InstallLlamaServer(req *ypb.InstallLlamaServerRequest, stream ypb.Yak_InstallLlamaServerServer) error {
	return s.InstallThirdPartyBinary(&ypb.InstallThirdPartyBinaryRequest{
		Name:  "llama-server",
		Proxy: req.GetProxy(),
		Force: true,
	}, stream)
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

	modelsDir := consts.GetDefaultAIModelDir()
	// 确保目录存在
	if err := os.MkdirAll(modelsDir, os.ModePerm); err != nil {
		return utils.Errorf("无法创建模型目录: %v", err)
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
	modelPath := consts.GetAIModelFilePath(targetModel.FileName)
	if modelPath == "" {
		return utils.Errorf("模型文件不存在: %s", targetModel.FileName)
	}

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

type BuildInVectorDBLink struct {
	Description string
	DownloadURL string
}

var buildInVectorDBLink = map[string]BuildInVectorDBLink{
	plugins_rag.PLUGIN_RAG_COLLECTION_NAME: {
		Description: "插件向量数据库，包含所有插件的向量数据，可以用于搜索插件",
		DownloadURL: "https://oss-qn.yaklang.com/yaklang-rag/plugins_rag.zip",
	},
}

func (s *Server) GetAllVectorStoreCollections(ctx context.Context, req *ypb.Empty) (*ypb.GetAllVectorStoreCollectionsResponse, error) {
	collections := []string{}
	for collectionName := range buildInVectorDBLink {
		collections = append(collections, collectionName)
	}
	sort.Strings(collections)

	collectionsPB := []*ypb.VectorStoreCollection{}
	for _, collection := range collections {
		info := buildInVectorDBLink[collection]
		collectionsPB = append(collectionsPB, &ypb.VectorStoreCollection{
			Name:        collection,
			Description: info.Description,
		})
	}
	return &ypb.GetAllVectorStoreCollectionsResponse{
		Collections: collectionsPB,
	}, nil
}

func (s *Server) IsSearchVectorDatabaseReady(ctx context.Context, req *ypb.IsSearchVectorDatabaseReadyRequest) (*ypb.IsSearchVectorDatabaseReadyResponse, error) {
	notReadyCollectionNames := []string{}
	db := consts.GetGormProfileDatabase()
	for _, collectionName := range req.GetCollectionNames() {
		if !rag.IsReadyCollection(db, collectionName) {
			notReadyCollectionNames = append(notReadyCollectionNames, collectionName)
		}
	}

	return &ypb.IsSearchVectorDatabaseReadyResponse{
		IsReady:                 len(notReadyCollectionNames) == 0,
		NotReadyCollectionNames: notReadyCollectionNames,
	}, nil
}
func (s *Server) DeleteSearchVectorDatabase(ctx context.Context, req *ypb.DeleteSearchVectorDatabaseRequest) (*ypb.GeneralResponse, error) {
	collectionNames := req.GetCollectionNames()
	errs := []error{}
	for _, collectionName := range collectionNames {
		err := rag.RemoveCollection(consts.GetGormProfileDatabase(), collectionName)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return &ypb.GeneralResponse{
		Ok: len(errs) == 0,
	}, utils.JoinErrors(errs...)
}
func (s *Server) InitSearchVectorDatabase(req *ypb.InitSearchVectorDatabaseRequest, stream ypb.Yak_InitSearchVectorDatabaseServer) error {
	collectionNames := req.GetCollectionNames()
	for _, collectionName := range collectionNames {
		if _, ok := buildInVectorDBLink[collectionName]; !ok {
			return utils.Errorf("不支持的集合类型: %s", collectionName)
		}
	}

	for _, collection := range collectionNames {
		stream.Send(&ypb.ExecResult{
			IsMessage: true,
			Message:   []byte(fmt.Sprintf("开始下载 %s 向量数据", collection)),
		})
		tmpFileName := fmt.Sprintf("tmp_%s.zip", collection)
		err := s.DownloadWithStream(req.GetProxy(), func() (urlStr string, name string, err error) {
			return buildInVectorDBLink[collection].DownloadURL, tmpFileName, nil
		}, stream)
		if err != nil {
			return err
		}

		stream.Send(&ypb.ExecResult{
			IsMessage: true,
			Message:   []byte(fmt.Sprintf("下载完成，开始导入 %s 向量数据", collection)),
		})

		zipPath := filepath.Join(consts.GetDefaultYakitProjectsDir(), "libs", tmpFileName)
		db := consts.GetGormProfileDatabase()
		err = rag.ImportVectorDataFullUpdate(db, zipPath)
		if err != nil {
			return err
		}
		os.Remove(zipPath)
		stream.Send(&ypb.ExecResult{
			IsMessage: true,
			Message:   []byte(fmt.Sprintf("导入 %s 向量数据完成，已删除临时文件 %s", collection, zipPath)),
		})
	}

	return nil
}
