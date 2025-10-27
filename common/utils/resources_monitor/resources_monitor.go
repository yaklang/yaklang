package resources_monitor

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type EmbedResourcesMonitor struct {
	resourceID  string
	buildInHash string
}

func NewEmbedResourcesMonitor(resourceID string, buildInHash string) *EmbedResourcesMonitor {
	return &EmbedResourcesMonitor{
		resourceID:  resourceID,
		buildInHash: buildInHash,
	}
}

func (m *EmbedResourcesMonitor) getCurrentHash(currentHashGetter func() string) string {
	if consts.IsDevMode() {
		return m.buildInHash
	} else {
		return currentHashGetter()
	}
}

func (m *EmbedResourcesMonitor) MonitorModifiedWithAction(currentHashGetter func() string, callBack func() error) error {
	resourceHash := m.getCurrentHash(currentHashGetter)
	if resourceHash != yakit.Get(m.resourceID) {
		err := callBack()
		if err != nil {
			return err
		} else {
			yakit.Set(m.resourceID, resourceHash)
		}
	}
	return nil
}
