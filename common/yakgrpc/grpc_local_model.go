package yakgrpc

import (
	"context"
	"encoding/json"
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
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var USER_CUSTOM_LOCAL_MODEL_KEY = "USER_CUSTOM_LOCAL_MODEL"

// CustomLocalModel 自定义本地模型结构
type CustomLocalModel struct {
	Name        string `json:"name"`
	ModelType   string `json:"model_type"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

// RunningModelInfo 运行中模型的信息
type RunningModelInfo struct {
	Cmd       *exec.Cmd
	Host      string
	Port      int32
	ModelType string
}

// LocalModelManager 本地模型管理器
type LocalModelManager struct {
	mutex         sync.RWMutex
	runningModels map[string]*RunningModelInfo
}

var localModelManager = &LocalModelManager{
	runningModels: make(map[string]*RunningModelInfo),
}

// getCustomModelsFromDB 从数据库获取自定义模型列表
func getCustomModelsFromDB() ([]*ypb.LocalModelConfig, error) {
	modelsJSON := yakit.GetKey(consts.GetGormProfileDatabase(), USER_CUSTOM_LOCAL_MODEL_KEY)
	if modelsJSON == "" {
		return []*ypb.LocalModelConfig{}, nil
	}

	var customModels []CustomLocalModel
	err := json.Unmarshal([]byte(modelsJSON), &customModels)
	if err != nil {
		return nil, err
	}

	var result []*ypb.LocalModelConfig
	for _, model := range customModels {
		result = append(result, &ypb.LocalModelConfig{
			Name:        model.Name,
			Type:        model.ModelType,
			FileName:    filepath.Base(model.Path), // 从路径中提取文件名
			DownloadURL: "",                        // 自定义模型没有下载链接
			Description: model.Description,
			IsLocal:     true,
			IsReady:     utils.FileExists(model.Path),
			Path:        model.Path,
			DefaultPort: 8080, // 默认端口
		})
	}

	return result, nil
}
func getModelTypeByTags(tags ...string) string {
	typeMap := map[string]string{
		"embedding":      "embedding",
		"aichat":         "aichat",
		"speech-to-text": "speech-to-text",
	}

	for _, tag := range tags {
		if _, ok := typeMap[tag]; ok {
			return typeMap[tag]
		}
	}
	return ""
}

// getSupportedModels 获取支持的模型列表（包括内置和自定义）
func getSupportedModels() []*ypb.LocalModelConfig {
	// 内置模型
	aimodelNames := thirdparty_bin.GetBinaryNamesByTags("aimodel")
	allBins := thirdparty_bin.GetRegisteredBinaries()

	builtinModels := []*ypb.LocalModelConfig{}

	for _, name := range aimodelNames {
		bin, ok := allBins[name]
		if !ok {
			continue
		}
		downloadUrl, err := thirdparty_bin.GetDownloadInfo(name)
		if err != nil {
			log.Errorf("获取模型下载信息失败: %v", err)
			continue
		}

		var binPath string

		isInstalled := false
		status, err := thirdparty_bin.GetStatus(name)
		if err != nil {
			isInstalled = false
		} else {
			isInstalled = status.Installed
		}

		if isInstalled {
			binPath, err = thirdparty_bin.GetBinaryPath(name)
			if err != nil {
				binPath = ""
			}
		}

		builtinModels = append(builtinModels, &ypb.LocalModelConfig{
			Name:        bin.Name,
			Type:        getModelTypeByTags(bin.Tags...),
			DownloadURL: downloadUrl.URL,
			Description: bin.Description,
			Path:        binPath,
			IsReady:     isInstalled,
			IsLocal:     false,
			DefaultPort: 8080,
		})
	}

	// 获取自定义模型
	customModels, err := getCustomModelsFromDB()
	if err != nil {
		log.Errorf("获取自定义模型失败: %v", err)
		return builtinModels
	}

	// 合并内置模型和自定义模型
	allModels := append(builtinModels, customModels...)
	return allModels
}

// GetSupportedLocalModels 获取支持的本地模型列表
func (s *Server) GetSupportedLocalModels(ctx context.Context, req *ypb.Empty) (*ypb.GetSupportedLocalModelsResponse, error) {
	return &ypb.GetSupportedLocalModelsResponse{
		Models: getSupportedModels(),
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

	// 检查模型是否就绪
	if targetModel.IsLocal {
		if utils.FileExists(targetModel.Path) {
			return &ypb.IsLocalModelReadyResponse{
				Ok:     true,
				Reason: "",
			}, nil
		} else {
			return &ypb.IsLocalModelReadyResponse{
				Ok:     false,
				Reason: fmt.Sprintf("模型文件不存在: %s", targetModel.Path),
			}, nil
		}
	} else {
		status, err := thirdparty_bin.GetStatus(targetModel.Name)
		if err != nil {
			return &ypb.IsLocalModelReadyResponse{
				Ok:     false,
				Reason: fmt.Sprintf("获取模型安装状态错误: %s", err.Error()),
			}, nil
		} else {
			if status.Installed {
				return &ypb.IsLocalModelReadyResponse{
					Ok:     true,
					Reason: "",
				}, nil
			} else {
				return &ypb.IsLocalModelReadyResponse{
					Ok:     false,
					Reason: fmt.Sprintf("模型未安装: %s", targetModel.Name),
				}, nil
			}
		}
	}
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
	return s.InstallThirdPartyBinary(&ypb.InstallThirdPartyBinaryRequest{
		Name:  req.GetModelName(),
		Proxy: req.GetProxy(),
		Force: true,
	}, stream)
}

// StartLocalModel 启动本地模型
func (s *Server) StartLocalModel(req *ypb.StartLocalModelRequest, stream ypb.Yak_StartLocalModelServer) error {
	modelName := req.GetModelName()
	if modelName == "" {
		return utils.Error("模型名称不能为空")
	}

	// 检查模型是否已在运行
	localModelManager.mutex.RLock()
	if cmd, exists := localModelManager.runningModels[modelName]; exists && cmd.Cmd.ProcessState == nil {
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
	localModelManager.runningModels[modelName] = &RunningModelInfo{
		Cmd:       cmd,
		Host:      host,
		Port:      port,
		ModelType: targetModel.Type,
	}
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

func (s *Server) AddLocalModel(ctx context.Context, req *ypb.AddLocalModelRequest) (*ypb.GeneralResponse, error) {
	// 参数验证
	if req.GetName() == "" {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Error("模型名称不能为空")
	}

	if req.GetPath() == "" {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Error("模型路径不能为空")
	}

	// 检查模型文件是否存在
	if exists, _ := utils.PathExists(req.GetPath()); !exists {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("模型文件不存在: %s", req.GetPath())
	}

	// 获取现有模型列表
	modelsJSON := yakit.GetKey(consts.GetGormProfileDatabase(), USER_CUSTOM_LOCAL_MODEL_KEY)
	var existingModels []CustomLocalModel
	if modelsJSON != "" {
		err := json.Unmarshal([]byte(modelsJSON), &existingModels)
		if err != nil {
			return &ypb.GeneralResponse{
				Ok: false,
			}, utils.Errorf("解析现有模型列表失败: %v", err)
		}
	}

	// 检查模型名称是否已存在
	for _, model := range existingModels {
		if model.Name == req.GetName() {
			return &ypb.GeneralResponse{
				Ok: false,
			}, utils.Errorf("模型名称已存在: %s", req.GetName())
		}
	}

	// 创建新模型
	newModel := CustomLocalModel{
		Name:        req.GetName(),
		ModelType:   req.GetModelType(),
		Description: req.GetDescription(),
		Path:        req.GetPath(),
	}

	// 添加到现有列表
	existingModels = append(existingModels, newModel)

	// 序列化并保存
	updatedJSON, err := json.Marshal(existingModels)
	if err != nil {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("序列化模型列表失败: %v", err)
	}

	yakit.SetKey(consts.GetGormProfileDatabase(), USER_CUSTOM_LOCAL_MODEL_KEY, string(updatedJSON))

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) DeleteLocalModel(ctx context.Context, req *ypb.DeleteLocalModelRequest) (*ypb.GeneralResponse, error) {
	// 参数验证
	if req.GetName() == "" {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Error("模型名称不能为空")
	}

	// 判断是否是通过 Yakit 安装的模型
	status, err := thirdparty_bin.GetStatus(req.GetName())
	if err == nil {
		if !status.Installed {
			return &ypb.GeneralResponse{
				Ok: false,
			}, utils.Errorf("模型 %s 未下载，请先下载", req.GetName())
		}
		err = thirdparty_bin.Uninstall(req.GetName())
		if err != nil {
			return &ypb.GeneralResponse{
				Ok: false,
			}, utils.Errorf("卸载模型 %s 失败: %v", req.GetName(), err)
		}
		return &ypb.GeneralResponse{
			Ok: true,
		}, nil
	}

	// 获取现有模型列表
	modelsJSON := yakit.GetKey(consts.GetGormProfileDatabase(), USER_CUSTOM_LOCAL_MODEL_KEY)
	if modelsJSON == "" {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("模型不存在: %s", req.GetName())
	}

	var existingModels []CustomLocalModel
	err = json.Unmarshal([]byte(modelsJSON), &existingModels)
	if err != nil {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("解析现有模型列表失败: %v", err)
	}

	// 查找并删除指定模型
	var updatedModels []CustomLocalModel
	var deletedModel *CustomLocalModel
	found := false

	for _, model := range existingModels {
		if model.Name == req.GetName() {
			deletedModel = &model
			found = true
			continue // 跳过这个模型，相当于删除
		}
		updatedModels = append(updatedModels, model)
	}

	if !found {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("模型不存在: %s", req.GetName())
	}

	// 如果请求删除源文件
	if req.GetDeleteSourceFile() && deletedModel != nil {
		if exists, _ := utils.PathExists(deletedModel.Path); exists {
			err := os.Remove(deletedModel.Path)
			if err != nil {
				log.Warnf("删除模型文件失败: %v", err)
				// 不要因为文件删除失败而返回错误，继续删除数据库记录
			}
		}
	}

	// 序列化并保存更新后的列表
	updatedJSON, err := json.Marshal(updatedModels)
	if err != nil {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("序列化模型列表失败: %v", err)
	}

	yakit.SetKey(consts.GetGormProfileDatabase(), USER_CUSTOM_LOCAL_MODEL_KEY, string(updatedJSON))

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) UpdateLocalModel(ctx context.Context, req *ypb.UpdateLocalModelRequest) (*ypb.GeneralResponse, error) {
	// 参数验证
	if req.GetName() == "" {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Error("模型名称不能为空")
	}

	// 获取现有模型列表
	modelsJSON := yakit.GetKey(consts.GetGormProfileDatabase(), USER_CUSTOM_LOCAL_MODEL_KEY)
	if modelsJSON == "" {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("模型不存在: %s", req.GetName())
	}

	var existingModels []CustomLocalModel
	err := json.Unmarshal([]byte(modelsJSON), &existingModels)
	if err != nil {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("解析现有模型列表失败: %v", err)
	}

	// 查找指定模型
	var updatedModels []CustomLocalModel
	found := false

	for _, model := range existingModels {
		if model.Name == req.GetName() {
			found = true
			// 更新模型信息，只更新非空字段
			updatedModel := model

			if req.GetModelType() != "" {
				updatedModel.ModelType = req.GetModelType()
			}

			if req.GetDescription() != "" {
				updatedModel.Description = req.GetDescription()
			}

			if req.GetPath() != "" {
				// 验证新路径是否存在
				if exists, _ := utils.PathExists(req.GetPath()); !exists {
					return &ypb.GeneralResponse{
						Ok: false,
					}, utils.Errorf("模型文件不存在: %s", req.GetPath())
				}
				updatedModel.Path = req.GetPath()
			}

			updatedModels = append(updatedModels, updatedModel)
		} else {
			updatedModels = append(updatedModels, model)
		}
	}

	if !found {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("模型不存在: %s", req.GetName())
	}

	// 序列化并保存更新后的列表
	updatedJSON, err := json.Marshal(updatedModels)
	if err != nil {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("序列化模型列表失败: %v", err)
	}

	yakit.SetKey(consts.GetGormProfileDatabase(), USER_CUSTOM_LOCAL_MODEL_KEY, string(updatedJSON))

	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) GetAllStartedLocalModels(ctx context.Context, req *ypb.Empty) (*ypb.GetAllStartedLocalModelsResponse, error) {
	localModelManager.mutex.RLock()
	defer localModelManager.mutex.RUnlock()

	var models []*ypb.StartedLocalModelInfo

	// 遍历所有运行中的模型
	for modelName, cmdInfo := range localModelManager.runningModels {
		// 检查进程是否还在运行
		if cmdInfo.Cmd.ProcessState == nil || !cmdInfo.Cmd.ProcessState.Exited() {
			// 检查模型类型是否为aichat
			if cmdInfo.ModelType == "aichat" {
				// 创建StartedLocalModelInfo结构体
				modelInfo := &ypb.StartedLocalModelInfo{
					Name:      modelName,
					ModelType: cmdInfo.ModelType,
					Host:      cmdInfo.Host, // 从RunningModelInfo获取主机
					Port:      cmdInfo.Port, // 从RunningModelInfo获取端口
				}
				models = append(models, modelInfo)
			}
		}
	}

	return &ypb.GetAllStartedLocalModelsResponse{
		Models: models,
	}, nil
}

// 清除所有本地模型
func (s *Server) ClearAllModels(ctx context.Context, req *ypb.ClearAllModelsRequest) (*ypb.GeneralResponse, error) {
	allModels := getSupportedModels()
	errors := []error{}
	for _, model := range allModels {
		resp, err := s.DeleteLocalModel(ctx, &ypb.DeleteLocalModelRequest{
			Name:             model.Name,
			DeleteSourceFile: req.GetDeleteSourceFile(),
		})
		if err != nil {
			errors = append(errors, err)
		}
		if !resp.GetOk() {
			errors = append(errors, utils.Errorf("删除模型 %s 失败", model.Name))
		}
	}
	return &ypb.GeneralResponse{
		Ok: len(errors) == 0,
	}, utils.JoinErrors(errors...)
}
