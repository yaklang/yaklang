package resources_monitor

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/builtinmetadata"
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
	if consts.IsDevMode() && !builtinmetadata.Enabled {
		return currentHashGetter()
	} else {
		return m.buildInHash
	}
}

func (m *EmbedResourcesMonitor) MonitorModifiedWithAction(currentHashGetter func() string, callBack func() error) error {
	resourceHash := m.getCurrentHash(currentHashGetter)
	if resourceHash == "" || resourceHash != yakit.Get(m.resourceID) {
		err := callBack()
		if err != nil {
			return err
		} else if resourceHash != "" && (!builtinmetadata.Enabled || currentHashGetter() != "") {
			yakit.Set(m.resourceID, resourceHash)
		}
	}
	return nil
}
