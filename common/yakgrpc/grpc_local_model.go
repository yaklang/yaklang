package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/localmodel"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"golang.org/x/exp/slices"
)

var USER_CUSTOM_LOCAL_MODEL_KEY = "USER_CUSTOM_LOCAL_MODEL"

// CustomLocalModel 自定义本地模型结构
type CustomLocalModel struct {
	Name        string `json:"name"`
	ModelType   string `json:"model_type"`
	Description string `json:"description"`
	Path        string `json:"path"`
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
		"chat":           "aichat",
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
	aimodelNames := thirdparty_bin.GetBinaryNamesByTags([]string{"aimodel"})
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
		modelType := getModelTypeByTags(bin.Tags...)
		if modelType == "" {
			modelType = "aichat"
		}
		builtinModels = append(builtinModels, &ypb.LocalModelConfig{
			Name:        bin.Name,
			Type:        modelType,
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
	manager := localmodel.GetManager()
	services := manager.ListServices()

	statusesMap := map[string]*ypb.LocalModelStatus{}
	for _, service := range services {
		log.Infof("found local model service: %v, %v", service.Config.Model, service.Status)
		statusesMap[service.Config.ModelPath] = &ypb.LocalModelStatus{
			Status:          service.Status.String(),
			Host:            service.Config.Host,
			Port:            service.Config.Port,
			Model:           service.Config.Model,
			ModelPath:       service.Config.ModelPath,
			LlamaServerPath: service.Config.LlamaServerPath,
			ContextSize:     int32(service.Config.ContextSize),
			ContBatching:    service.Config.ContBatching,
			BatchSize:       int32(service.Config.BatchSize),
			Threads:         int32(service.Config.Threads),
			Debug:           service.Config.Debug,
			Pooling:         service.Config.Pooling,
			StartupTimeout:  int32(service.Config.StartupTimeout.Seconds()),
			Args:            service.Config.Args,
		}
	}

	allSupportModels := getSupportedModels()
	allSupportModelsPB := []*ypb.LocalModelConfig{}
	allowModelTypes := []string{"embedding", "aichat"}
	for _, model := range allSupportModels {
		if !slices.Contains(allowModelTypes, model.Type) {
			continue
		}
		allSupportModelsPB = append(allSupportModelsPB, &ypb.LocalModelConfig{
			Name:        model.Name,
			Type:        model.Type,
			DownloadURL: model.DownloadURL,
			Description: model.Description,
			IsLocal:     model.IsLocal,
			IsReady:     model.IsReady,
			Path:        model.Path,
			DefaultPort: model.DefaultPort,
			Status:      statusesMap[model.Path],
		})
	}
	return &ypb.GetSupportedLocalModelsResponse{
		Models: allSupportModelsPB,
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
	manager := localmodel.GetManager()
	service, err := manager.GetServiceStatus(modelName)
	if err == nil && service != nil {
		if service.Status == localmodel.StatusRunning {
			return utils.Errorf("模型 %s 已在运行中", modelName)
		}
	}

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

	// 获取依赖路径信息
	llamaServerPath := consts.GetLlamaServerPath()
	modelPath := targetModel.Path

	// 构建启动命令
	opts := []localmodel.Option{
		localmodel.WithModelPath(modelPath),
		localmodel.WithModel(modelName),
		localmodel.WithLlamaServerPath(llamaServerPath),
		localmodel.WithDebug(true),
		localmodel.WithStartupTimeout(30 * time.Second),
	}

	if req.GetHost() != "" {
		opts = append(opts, localmodel.WithHost(req.GetHost()))
	}
	if req.GetPort() != 0 {
		opts = append(opts, localmodel.WithPort(req.GetPort()))
	}
	if req.GetContextSize() != 0 {
		opts = append(opts, localmodel.WithContextSize(int(req.GetContextSize())))
	}
	if req.GetBatchSize() != 0 {
		opts = append(opts, localmodel.WithBatchSize(int(req.GetBatchSize())))
	}
	if req.GetThreads() != 0 {
		opts = append(opts, localmodel.WithThreads(int(req.GetThreads())))
	}
	if req.GetDebug() {
		opts = append(opts, localmodel.WithDebug(true))
	}
	if req.GetPooling() != "" {
		opts = append(opts, localmodel.WithPooling(req.GetPooling()))
	}
	if req.GetStartupTimeoutMs() != 0 {
		opts = append(opts, localmodel.WithStartupTimeout(time.Duration(req.GetStartupTimeoutMs())*time.Millisecond))
	}
	if len(req.GetArgs()) > 0 {
		opts = append(opts, localmodel.WithArgs(req.GetArgs()...))
	}

	modelType := getModelTypeByTags(targetModel.Type)
	if modelType == "" {
		modelType = "aichat"
	}

	opts = append(opts, localmodel.WithModelType(modelType))
	stream.Send(&ypb.ExecResult{
		IsMessage: true,
		Message:   []byte(fmt.Sprintf("启动模型: %s，端口: %d", modelName, req.GetPort())),
	})

	err = manager.StartService(
		fmt.Sprintf("%s:%d", req.GetHost(), req.GetPort()),
		opts...,
	)
	if err != nil {
		return utils.Errorf("启动模型失败: %v", err)
	}

	stream.Send(&ypb.ExecResult{
		IsMessage: true,
		Message:   []byte(fmt.Sprintf("模型 %s 启动成功", modelName)),
	})

	return nil
}

func (s *Server) StopLocalModel(ctx context.Context, req *ypb.StopLocalModelRequest) (*ypb.GeneralResponse, error) {
	manager := localmodel.GetManager()
	modelName := req.GetModelName()
	var modelPath string
	allModels := getSupportedModels()
	for _, model := range allModels {
		if model.Name == modelName {
			modelPath = model.Path
			break
		}
	}
	if modelPath == "" {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("未找到模型: %s", modelName)
	}
	services := manager.ListServices()
	var service *localmodel.Service
	for _, s := range services {
		if s.Config.ModelPath == modelPath {
			service = s
			break
		}
	}
	if service == nil {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("未找到模型服务: %s", req.GetModelName())
	}
	err := manager.StopService(service.Name)
	return &ypb.GeneralResponse{
		Ok: err == nil,
	}, err
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

	modelType := req.GetModelType()
	modelType = getModelTypeByTags(modelType)
	if modelType == "" {
		return &ypb.GeneralResponse{
			Ok: false,
		}, utils.Errorf("不支持的模型类型: %s", req.GetModelType())
	}

	// 创建新模型
	newModel := CustomLocalModel{
		Name:        req.GetName(),
		ModelType:   modelType,
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
	localModelManager := localmodel.GetManager()
	services := localModelManager.ListServices()
	models := []*ypb.StartedLocalModelInfo{}
	for _, service := range services {
		if service.Status == localmodel.StatusRunning {
			models = append(models, &ypb.StartedLocalModelInfo{
				Name:      service.Config.Model,
				ModelType: service.Config.ModelType,
				Host:      service.Config.Host,
				Port:      service.Config.Port,
			})
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
			if strings.Contains(err.Error(), "未下载，请先下载") {
				continue
			}
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
