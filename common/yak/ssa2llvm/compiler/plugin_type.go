package compiler

import (
	"fmt"
	"strings"
)

const (
	YakPluginTypeYak      = "yak"
	YakPluginTypeCodec    = "codec"
	YakPluginTypePortScan = "port-scan"
	YakPluginTypeMitm     = "mitm"

	ssa2llvmCodecEntry    = "__ssa2llvm_codec_main"
	ssa2llvmPortScanEntry = "__ssa2llvm_portscan_main"
)

func applyCompilePluginType(cfg *CompileConfig) error {
	pluginType, err := normalizeYakPluginType(cfg.PluginType)
	if err != nil {
		return err
	}
	cfg.PluginType = pluginType

	switch pluginType {
	case "", YakPluginTypeYak:
		return nil
	case YakPluginTypeCodec:
		cfg.EntryFunctionName = ssa2llvmCodecEntry
	case YakPluginTypePortScan:
		cfg.EntryFunctionName = ssa2llvmPortScanEntry
	default:
		return fmt.Errorf("unsupported yak plugin type %q", pluginType)
	}
	return nil
}

func normalizeYakPluginType(pluginType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(pluginType)) {
	case "":
		return "", nil
	case YakPluginTypeYak, "native":
		return YakPluginTypeYak, nil
	case YakPluginTypeCodec:
		return YakPluginTypeCodec, nil
	case YakPluginTypePortScan, "portscan", "port_scan":
		return YakPluginTypePortScan, nil
	case YakPluginTypeMitm:
		return "", fmt.Errorf("yak plugin type %q is not supported by ssa2llvm yet", YakPluginTypeMitm)
	default:
		return "", fmt.Errorf("unknown yak plugin type %q", pluginType)
	}
}

func wrapYakPluginSource(code, pluginType string) (string, error) {
	switch strings.TrimSpace(pluginType) {
	case "", YakPluginTypeYak:
		return code, nil
	case YakPluginTypeCodec:
		return wrapCodecYakPluginSource(code), nil
	case YakPluginTypePortScan:
		return wrapPortScanYakPluginSource(code), nil
	default:
		return "", fmt.Errorf("unsupported yak plugin type %q", pluginType)
	}
}

func wrapCodecYakPluginSource(code string) string {
	return wrapYakPluginEntry(ssa2llvmCodecEntry, code, `
__ssa2llvm_param = cli.String("param", cli.setDefault(cli.String("input")))
cli.check()
__ssa2llvm_result = handle(__ssa2llvm_param)
println(__ssa2llvm_result)
return 0
`)
}

func wrapPortScanYakPluginSource(code string) string {
	return wrapYakPluginEntry(ssa2llvmPortScanEntry, code, `
__ssa2llvm_result = {
	"Target": cli.String("target"),
	"Port": cli.Int("port"),
	"State": cli.String("state", cli.setDefault("open")),
	"Reason": cli.String("reason"),
	"Fingerprint": {
		"ServiceName": cli.String("service"),
		"IP": cli.String("ip"),
		"Port": cli.Int("fp-port"),
		"Proto": cli.String("proto", cli.setDefault("tcp")),
	},
}
cli.check()
handle(__ssa2llvm_result)
return 0
`)
}

func wrapYakPluginEntry(entryName, code, body string) string {
	var b strings.Builder
	b.WriteString(entryName)
	b.WriteString(" = () => {\n")
	b.WriteString(code)
	if !strings.HasSuffix(code, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return b.String()
}
