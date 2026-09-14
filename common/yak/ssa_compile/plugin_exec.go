package ssa_compile

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// YakPluginExec runs a profile-DB yak plugin by name. Bound by
// ssa_compile_plugin so this package does not import yakscript
// (yakscript -> yak -> syntaxflow_scan would cycle if syntaxflow_scan
// imports ssa_compile).
type YakPluginExec func(ctx context.Context, pluginName string, para map[string]string, input string, handler func(*ypb.ExecResult) error) error

var yakPluginExec YakPluginExec

// BindYakPluginExec registers plugin execution used by auto-detect / plugin compile.
func BindYakPluginExec(fn YakPluginExec) {
	yakPluginExec = fn
}

func execYakPlugin(ctx context.Context, pluginName string, para map[string]string, input string, handler func(*ypb.ExecResult) error) error {
	if yakPluginExec == nil {
		return utils.Errorf("ssa compile plugin exec is not registered")
	}
	return yakPluginExec(ctx, pluginName, para, input, handler)
}
