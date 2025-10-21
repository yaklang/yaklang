package aimem

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (t *AIMemoryTriage) GetEmitter() *aicommon.Emitter {
	if t == nil {
		return nil
	}
	if t.invoker == nil {
		return nil
	}
	if config := t.invoker.GetConfig(); config != nil {
		return config.GetEmitter()
	}
	return nil
}

// HandleMemory 处理输入内容，自动构造记忆并去重保存
func (t *AIMemoryTriage) HandleMemory(i any) error {
	// 转换输入为字符串
	inputText := utils.InterfaceToString(i)
	if strings.TrimSpace(inputText) == "" {
		log.Infof("input is empty, skipping memory handling")
		return nil
	}

	log.Infof("handling memory for input: %s", utils.ShrinkString(inputText, 100))

	// 1. 使用 AddRawText 构造记忆实体
	entities, err := t.AddRawText(inputText)
	if err != nil {
		return utils.Errorf("failed to build memory entities: %v", err)
	}

	if len(entities) == 0 {
		log.Infof("no memory entities generated from input")
		return nil
	}

	for _, entity := range entities {
		if utils.IsNil(entity) {
			continue
		}
		if emitter := t.GetEmitter(); emitter != nil {
			emitter.EmitJSON(schema.EVENT_TYPE_MEMORY_BUILD, "memory-build", map[string]any{
				"memory_session_id": t.GetSessionID(),
				"memory":            entity,
			})
		}
	}

	log.Infof("generated %d memory entities from input", len(entities))

	// 2. 使用去重功能判断是否有重复
	worthSaving := t.ShouldSaveMemoryEntities(entities)

	// 3. 处理重复和非重复的记忆
	duplicateCount := len(entities) - len(worthSaving)
	if duplicateCount > 0 {
		log.Infof("detected %d duplicate memory entities, skipping them", duplicateCount)

		// 记录被跳过的重复记忆
		savedIds := make(map[string]bool)
		for _, saved := range worthSaving {
			savedIds[saved.Id] = true
		}

		for _, entity := range entities {
			if !savedIds[entity.Id] {
				log.Infof("skipping duplicate memory: %s (content: %s)",
					entity.Id, utils.ShrinkString(entity.Content, 50))
			}
		}
	}

	// 4. 保存非重复的记忆
	if len(worthSaving) > 0 {
		if err := t.SaveMemoryEntities(worthSaving...); err != nil {
			return utils.Errorf("failed to save memory entities: %v", err)
		}
		log.Infof("successfully saved %d new memory entities", len(worthSaving))
	} else {
		log.Infof("no new memories to save after deduplication")
	}

	return nil
}
