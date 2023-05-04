package node

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/spec/health"
)

func (n *NodeBase) heartbeat() {
	msg := n.NewBaseMessage(spec.MessageType_SystemMatrix)

	sysMatrix, err := health.NewSystemMatrixBase()
	sysMatrix.ExternalNetwork = n.ExternalIp
	if err != nil {
		log.Error(err)
		return
	}
	sysMatrix.NodeId = n.NodeId
	sysMatrix.HealthInfos = n.healthManager.GetHealthInfos()
	sysMatrix.NodeAliveDuration = uint64(n.healthManager.GetAliveDuration())

	raw, err := json.Marshal(sysMatrix)
	if err != nil {
		log.Errorf("marshal system matrix failed: %s, matrix: %v", err, spew.Sdump(sysMatrix))
		return
	}
	msg.Content = raw

	//n.Notify(spec.BackendKey_Heartbeat, msg)
	n.NotifyHeartbeat(spec.BackendKey_Heartbeat, msg)
}
