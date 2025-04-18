// automatically generated by stateify.

package channel

import (
	"context"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/state"
)

func (n *NotificationHandle) StateTypeName() string {
	return "pkg/tcpip/link/channel.NotificationHandle"
}

func (n *NotificationHandle) StateFields() []string {
	return []string{
		"n",
	}
}

func (n *NotificationHandle) beforeSave() {}

// +checklocksignore
func (n *NotificationHandle) StateSave(stateSinkObject state.Sink) {
	n.beforeSave()
	stateSinkObject.Save(0, &n.n)
}

func (n *NotificationHandle) afterLoad(context.Context) {}

// +checklocksignore
func (n *NotificationHandle) StateLoad(ctx context.Context, stateSourceObject state.Source) {
	stateSourceObject.Load(0, &n.n)
}

func (e *Endpoint) StateTypeName() string {
	return "pkg/tcpip/link/channel.Endpoint"
}

func (e *Endpoint) StateFields() []string {
	return []string{
		"LinkEPCapabilities",
		"SupportedGSOKind",
		"dispatcher",
		"linkAddr",
		"mtu",
		"q",
	}
}

func (e *Endpoint) beforeSave() {}

// +checklocksignore
func (e *Endpoint) StateSave(stateSinkObject state.Sink) {
	e.beforeSave()
	stateSinkObject.Save(0, &e.LinkEPCapabilities)
	stateSinkObject.Save(1, &e.SupportedGSOKind)
	stateSinkObject.Save(2, &e.dispatcher)
	stateSinkObject.Save(3, &e.linkAddr)
	stateSinkObject.Save(4, &e.mtu)
	stateSinkObject.Save(5, &e.q)
}

func (e *Endpoint) afterLoad(context.Context) {}

// +checklocksignore
func (e *Endpoint) StateLoad(ctx context.Context, stateSourceObject state.Source) {
	stateSourceObject.Load(0, &e.LinkEPCapabilities)
	stateSourceObject.Load(1, &e.SupportedGSOKind)
	stateSourceObject.Load(2, &e.dispatcher)
	stateSourceObject.Load(3, &e.linkAddr)
	stateSourceObject.Load(4, &e.mtu)
	stateSourceObject.Load(5, &e.q)
}

func init() {
	state.Register((*NotificationHandle)(nil))
	state.Register((*Endpoint)(nil))
}
