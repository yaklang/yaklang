package entitybase

import (
	"context"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"sync"
)

type endpointDataSignal struct {
	ch   chan struct{}
	data string
	once sync.Once
}

func newEndpointSignal() *endpointDataSignal {
	return &endpointDataSignal{
		ch:   make(chan struct{}),
		once: sync.Once{},
	}
}

func (s *endpointDataSignal) SetDataReady(data string) {
	s.once.Do(func() {
		s.data = data
		close(s.ch)
	})
}

func (s *endpointDataSignal) WaitDataReady(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-s.ch:
		return s.data, nil
	}
}

// SaveEndpoint  用于在局部分析中处理实体临时名到index的映射，以保证局部关系能实时准确地建立
type SaveEndpoint struct {
	ctx          context.Context
	eb           *EntityRepository
	nameToIndex  *omap.OrderedMap[string, string]
	nameSig      *omap.OrderedMap[string, *endpointDataSignal]
	entityFinish chan struct{}
	once         sync.Once
}

func (e *SaveEndpoint) SaveEntity(entity *schema.ERModelEntity) error {
	tempName := entity.EntityName
	saveEntity, err := e.eb.MergeAndSaveEntity(entity)
	if err != nil {
		return utils.Errorf("merge entity failed, %v", err)
	}
	e.nameToIndex.Set(tempName, saveEntity.Uuid)
	sig := e.nameSig.GetOrSet(tempName, newEndpointSignal())
	sig.SetDataReady(saveEntity.Uuid)
	return nil
}

func (e *SaveEndpoint) AddRelationship(sourceName, targetName, relationType, typeVerbose string, attr map[string]any) error {
	sourceIndex, err := e.WaitIndex(sourceName)
	if err != nil {
		return utils.Errorf("wait source entity index failed, %v", err)
	}
	targetIndex, err := e.WaitIndex(targetName)
	if err != nil {
		return utils.Errorf("wait target entity index failed, %v", err)
	}
	err = e.eb.AddRelationship(sourceIndex, targetIndex, relationType, typeVerbose, attr)
	if err != nil {
		return err
	}
	return nil
}

func (e *SaveEndpoint) WaitIndex(name string) (string, error) {
	sig := e.nameSig.GetOrSet(name, newEndpointSignal())
	select {
	case <-e.entityFinish:
		index := uuid.New().String()
		currentIndex := e.nameToIndex.GetOrSet(name, index)
		var err error
		if currentIndex == index { // 如果Set了，说明之前没有这个实体，需要创建
			err = e.eb.CreateEntity(&schema.ERModelEntity{
				EntityName: name,
				Uuid:       index,
			})
		}

		return currentIndex, err
	case <-sig.ch:
		return sig.data, nil
	case <-e.ctx.Done():
		return "", utils.Errorf("wait entity index context done, %v", e.ctx.Err())
	}
}

func (e *SaveEndpoint) FinishEntitySave() {
	e.once.Do(func() {
		close(e.entityFinish)
	})
}
