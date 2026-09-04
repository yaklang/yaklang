package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/aiforge/browsercrypto"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type serverBrowserExtensionBridge struct {
	server *Server
}

func (b serverBrowserExtensionBridge) Available() bool {
	return b.server != nil && b.server.browserBridge != nil
}

func (b serverBrowserExtensionBridge) CallDevice(
	ctx context.Context,
	deviceID string,
	method string,
	params interface{},
) (json.RawMessage, error) {
	if !b.Available() {
		return nil, errors.New("browser extension bridge is not running")
	}
	return b.server.browserBridge.CallDevice(ctx, deviceID, method, params)
}

func (b serverBrowserExtensionBridge) CapabilityCatalog(
	deviceID string,
) (*browser.ExtensionBridgeCapabilityCatalog, bool) {
	if !b.Available() {
		return nil, false
	}
	for _, connection := range b.server.browserBridge.Snapshot().Connections {
		if connection.DeviceID == deviceID && connection.CapabilityCatalog != nil {
			return connection.CapabilityCatalog, true
		}
	}
	return nil, false
}

func (s *Server) registerRuntimeForges() error {
	if s == nil || s.runtimeForges == nil {
		return errors.New("runtime forge registry is not initialized")
	}
	cryptoRunner := browsercrypto.NewRunner(serverBrowserExtensionBridge{server: s})
	return s.runtimeForges.RegisterWithReAct(
		browsercrypto.ForgeName,
		cryptoRunner.Execute,
		cryptoRunner.PrepareReAct,
	)
}

func normalizeRuntimeForgeParams(
	params []*ypb.ExecParamItem,
	userQuery string,
) []*ypb.ExecParamItem {
	if params == nil && userQuery != "" {
		return []*ypb.ExecParamItem{{Key: "query", Value: userQuery}}
	}
	return params
}

func (s *Server) executeRuntimeForge(
	name string,
	ctx context.Context,
	params []*ypb.ExecParamItem,
	userQuery string,
	options ...aicommon.ConfigOption,
) (*aiforge.ForgeResult, bool, error) {
	if s == nil || s.runtimeForges == nil {
		return nil, false, nil
	}
	params = normalizeRuntimeForgeParams(params, userQuery)
	return s.runtimeForges.Execute(name, ctx, params, options...)
}

func (s *Server) prepareRuntimeForgeReAct(
	name string,
	ctx context.Context,
	params []*ypb.ExecParamItem,
	userQuery string,
) (*aiforge.RuntimeForgeReActPreparation, bool, error) {
	if s == nil || s.runtimeForges == nil {
		return nil, false, nil
	}
	params = normalizeRuntimeForgeParams(params, userQuery)
	return s.runtimeForges.PrepareReAct(name, ctx, params)
}
