package aicommon

import (
	"bytes"
	"fmt"
	"os"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

const (
	CONTEXT_PROVIDER_TYPE_FILE           = "file"
	CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE = "knowledge_base"
	CONTEXT_PROVIDER_TYPE_AITOOL         = "aitool"
	CONTEXT_PROVIDER_TYPE_AIFORGE        = "aiforge"
)

type ContextProviderEntry struct {
	Name     string
	Provider ContextProvider
	Traced   bool
}

type ContextProvider func(config AICallerConfigIf, emitter *Emitter, key string) (string, error)

func FileContextProvider(filePath string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		contentBytes, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
		}
		content := string(contentBytes)
		content = utils.ShrinkString(content, 200)
		return fmt.Sprintf("User Prompt: %s File: %s\nContent:\n%s", userPrompt, filePath, content), nil
	}
}

func KnowledgeBaseContextProvider(knowledgeBaseName string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		knowledgeBase, err := yakit.GetKnowledgeBaseByName(consts.GetGormProfileDatabase(), knowledgeBaseName)
		if err != nil {
			return "", utils.Errorf("failed to get knowledge base %s: %w", knowledgeBaseName, err)
		}
		var infoBuffer bytes.Buffer
		infoBuffer.WriteString("============== Knowledge Base Info ==============\n")
		infoBuffer.WriteString(fmt.Sprintf("Name: %s\n", knowledgeBaseName))
		infoBuffer.WriteString(fmt.Sprintf("Description: %s\n", knowledgeBase.KnowledgeBaseDescription))
		infoBuffer.WriteString(fmt.Sprintf("Type: %s\n", knowledgeBase.KnowledgeBaseType))
		infoBuffer.WriteString(fmt.Sprintf("Tags: %s\n", knowledgeBase.Tags))
		infoBuffer.WriteString("\n============== Important Instructions ==============\n")
		infoBuffer.WriteString("【重要提示】用户已附加此知识库作为问题的参考资源。\n")
		infoBuffer.WriteString("当用户问及与此知识库相关的内容时，请务必使用 `knowledge_enhance_answer` action 或相关知识库查询工具来检索知识库内容。\n")
		infoBuffer.WriteString(fmt.Sprintf("查询时请指定知识库名称为: %s\n", knowledgeBaseName))
		infoBuffer.WriteString("请基于知识库查询结果来回答用户的问题，确保答案准确且有据可依。\n")
		infoBuffer.WriteString("在回答时，请明确引用知识库中的相关信息。\n")
		return fmt.Sprintf("User Prompt: %s\n%s", userPrompt, infoBuffer.String()), nil
	}
}

func AIToolContextProvider(aitoolName string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		aitool, err := config.GetAiToolManager().GetToolByName(aitoolName)
		if err != nil {
			return "", utils.Errorf("failed to get aitool %s: %w", aitoolName, err)
		}
		var infoBuffer bytes.Buffer
		infoBuffer.WriteString("============== AITool Info ==============\n")
		infoBuffer.WriteString(fmt.Sprintf("Name: %s\n", aitool.Name))
		infoBuffer.WriteString(fmt.Sprintf("Description: %s\n", aitool.Description))
		infoBuffer.WriteString(fmt.Sprintf("Schema: %s\n", aitool.ToJSONSchemaString()))
		infoBuffer.WriteString("\n============== Important Instructions ==============\n")
		infoBuffer.WriteString("【重要提示】用户已指定使用此工具来完成任务。\n")
		infoBuffer.WriteString(fmt.Sprintf("请优先调用工具 '%s' 来解决用户的问题。\n", aitool.Name))
		infoBuffer.WriteString("在执行任务时，请根据上述工具的Schema正确传入参数。\n")
		infoBuffer.WriteString("如果此工具无法完全满足需求，可以结合其他工具辅助完成，但应以此工具为主。\n")
		return fmt.Sprintf("User Prompt: %s\n%s", userPrompt, infoBuffer.String()), nil
	}
}

func AIForgeContextProvider(aiforgeName string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		aiforge, err := yakit.GetAIForgeByName(consts.GetGormProfileDatabase(), aiforgeName)
		if err != nil {
			return "", utils.Errorf("failed to get aiforge %s: %w", aiforgeName, err)
		}
		var infoBuffer bytes.Buffer
		infoBuffer.WriteString("============== AIForge Info ==============\n")
		infoBuffer.WriteString(fmt.Sprintf("Name: %s\n", aiforge.ForgeName))
		infoBuffer.WriteString(fmt.Sprintf("Description: %s\n", aiforge.Description))
		infoBuffer.WriteString(fmt.Sprintf("Params: %s\n", aiforge.Params))
		infoBuffer.WriteString("\n============== Important Instructions ==============\n")
		infoBuffer.WriteString("【重要提示】用户已指定使用此AI蓝图(Forge)来完成任务。\n")
		infoBuffer.WriteString(fmt.Sprintf("请优先调用AI蓝图 '%s' 来解决用户的问题。\n", aiforge.ForgeName))
		infoBuffer.WriteString("此蓝图专门设计用于处理特定类型的任务，能够提供更专业和高效的解决方案。\n")
		infoBuffer.WriteString("在执行时，请根据上述参数Schema正确配置蓝图参数，确保任务顺利完成。\n")
		infoBuffer.WriteString("如果蓝图执行过程中遇到问题，请及时向用户反馈并寻求进一步指导。\n")
		return fmt.Sprintf("User Prompt: %s\n%s", userPrompt, infoBuffer.String()), nil
	}
}

func NewContextProvider(typ string, data string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		switch typ {
		case CONTEXT_PROVIDER_TYPE_FILE:
			return FileContextProvider(data, userPrompt...)(config, emitter, key)
		case CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE:
			return KnowledgeBaseContextProvider(data, userPrompt...)(config, emitter, key)
		case CONTEXT_PROVIDER_TYPE_AITOOL:
			return AIToolContextProvider(data, userPrompt...)(config, emitter, key)
		case CONTEXT_PROVIDER_TYPE_AIFORGE:
			return AIForgeContextProvider(data, userPrompt...)(config, emitter, key)
		}
		return "", utils.Errorf("unknown context provider type: %s", typ)
	}
}

type ContextProviderManager struct {
	maxBytes int
	m        sync.RWMutex
	callback *omap.OrderedMap[string, ContextProvider]
}

func NewContextProviderManager() *ContextProviderManager {
	return &ContextProviderManager{
		maxBytes: 10 * 1024, // 10KB
		callback: omap.NewOrderedMap(make(map[string]ContextProvider)),
	}
}

func (r *ContextProviderManager) RegisterTracedContent(name string, cb ContextProvider) {
	var m = new(sync.Mutex)
	var firstCall = utils.NewOnce()
	var lastErr error
	var lastContent string
	var buf bytes.Buffer

	update := func(content string, newErr error) string {
		m.Lock()
		defer m.Unlock()
		var result string
		firstCall.DoOr(func() {
			lastContent = content
			lastErr = newErr
			buf.Reset()
		}, func() {
			var diffResult string
			var err error
			if lastContent != "" && content != "" {
				diffResult, err = yakdiff.DiffToString(lastContent, content)
				if err != nil {
					log.Warnf("diff to string failed: %v", err)
				}
			} else if lastContent == "" {
				diffResult = "last-content is empty, new content added"
			}

			if newErr == nil && lastErr != nil {
				diffResult += fmt.Sprintf("\n[Error resolved: %v]", lastErr)
			} else if newErr != nil && lastErr == nil {
				diffResult += fmt.Sprintf("\n[New error occurred: %v]", newErr)
			} else if newErr != nil && lastErr != nil && newErr.Error() != lastErr.Error() {
				diffResult += fmt.Sprintf("\n[Error changed from: %v to: %v]", lastErr, newErr)
			}

			diff, err := utils.RenderTemplate(`<|CHANGES_DIFF_{{ .nonce }}|>
{{ .diff }}
<|CHANGES_DIFF_{{ .nonce }}|>`, map[string]any{
				"diff":  diffResult,
				"nonce": utils.RandStringBytes(4),
			})
			if err != nil {
				log.Warnf("render template failed: %v", err)
			} else {
				buf.WriteString(diff)
				buf.WriteString("\n")
			}
			result = buf.String()
			lastContent = content
			lastErr = newErr
			buf.Reset()
		})
		return result
	}

	wrapper := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		result, err := cb(config, emitter, key)
		extra := update(result, err)
		if err != nil {
			if extra == "" {
				return result, err
			}
			return result + "\n\n" + extra + "", err
		}
		log.Infof("ContextProvider %s result: %s", name, utils.ShrinkString(result, 200))
		if extra == "" {
			return result, nil
		}
		return result + "\n\n" + extra, nil
	}
	r.Register(name, wrapper)
}

func (r *ContextProviderManager) Register(name string, cb ContextProvider) {
	r.m.Lock()
	defer r.m.Unlock()
	if r.callback.Have(name) {
		log.Warnf("context provider %s already registered, ignore, if you want to use new callback, unregister first", name)
		return
	}
	r.callback.Set(name, func(config AICallerConfigIf, emitter *Emitter, key string) (_ string, finalErr error) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("context provider %s panic: %v", name, err)
				utils.PrintCurrentGoroutineRuntimeStack()
				finalErr = utils.Errorf("context provider %s panic: %v", name, err)
			}
		}()
		return cb(config, emitter, key)
	})
}

func (r *ContextProviderManager) Unregister(name string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.callback.Delete(name)
}

func (r *ContextProviderManager) Execute(config AICallerConfigIf, emitter *Emitter) string {
	r.m.RLock()
	defer r.m.RUnlock()

	if r.callback.Len() == 0 {
		return ""
	}

	var buf bytes.Buffer
	r.callback.ForEach(func(name string, cb ContextProvider) bool {
		result, err := cb(config, emitter, name)
		if err != nil {
			result = `[Error getting context: ` + err.Error() + `]`
		}
		flag := utils.RandStringBytes(4)
		buf.WriteString(fmt.Sprintf("<|AUTO_PROVIDE_CTX_[%v]_START key=%v|>\n", flag, name))
		buf.WriteString(result)
		buf.WriteString(fmt.Sprintf("\n<|AUTO_PROVIDE_CTX_[%v]_END|>", flag))
		return true
	})

	result := buf.String()
	if len(result) > r.maxBytes {
		shrinkSize := int(float64(r.maxBytes) * 0.8)
		result = utils.ShrinkString(result, shrinkSize)
		log.Warnf("context provider result exceeded maxBytes (%d), shrunk to %d characters", r.maxBytes, shrinkSize)
	}

	return result
}
