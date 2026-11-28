package main

import (
	"fmt"
	"strings"
)

// DefaultProfiles maps built-in profile names to receiver symbol substring
// patterns whose unused reflect-only exported methods may be pruned by the
// patched Go linker.
//
// Rules:
//   - A plain entry (e.g. "/parser.") excludes matching receiver types from
//     reflectSeen-forced retention. Direct calls and interface-satisfying
//     methods are NEVER pruned.
//   - An entry prefixed with "!" is an explicit allowlist override that always
//     keeps matching receiver types reflectable.
//
// Safety invariants:
//   - NEVER include Yak script runtime / stdlib packages (yaklib, ssaapi,
//     schema, ypb, goja.Runtime, net/http, etc.) in default profiles.
var DefaultProfiles = map[string][]string{
	// tier1: Pure generated ANTLR / AST parser packages. Zero Yak script
	// reflection surface.
	"tier1": {
		"/parser.",
		"common/syntaxflow/sf.",
		"common/yak/typescript/frontend/ast.",
		"github.com/yaklang/antlr/v4.",
	},

	// tier2: tier1 + heavy internal engine/protocol/data-structure packages
	// whose exported methods are called directly or via interfaces, not via
	// Yak script reflect.MethodByName or yak2ssa extern type inspection.
	//
	// Explicitly EXCLUDED from pruning (kept reflectable because yaklib,
	// builtin AI tools, or core plugins inspect/reflect them):
	//   - common/yak/ssaapi. (*ssaapi.Program, *ssaapi.SyntaxFlowResult)
	//   - github.com/yaklang/goja (js.New() -> *goja.Runtime)
	//   - common/pcapx, github.com/gopacket/gopacket (pcap yaklib / AI tools)
	//   - common/lowtun/netstack/gvisor (lowtun / Tun core plugin)
	//   - github.com/yaklang/javajive (yso / Shiro core plugin)
	//   - common/utils/omap, common/utils/chanx
	"tier2": {
		// tier1
		"/parser.",
		"common/syntaxflow/sf.",
		"common/yak/typescript/frontend/ast.",
		"github.com/yaklang/antlr/v4.",
		// internal SSA IR (distinct from ssaapi)
		"common/yak/ssa.",
		// internal binary / IDS / cache / browser-automation internals
		"common/bin-parser",
		"common/suricata",
		"common/utils/dbcache",
		"github.com/go-rod/rod",
		// explicit safety guards (script-reflected types)
		"!common/yak/ssaapi.",
		"!github.com/yaklang/goja",
		"!common/pcapx",
		"!github.com/gopacket/gopacket",
		"!common/lowtun/netstack/gvisor",
		"!github.com/yaklang/javajive",
	},
}

// ResolveProfilePatterns returns the comma-separated pattern string for a
// named profile. "off" or "" returns empty string (no pruning).
func ResolveProfilePatterns(profile string) (string, error) {
	p := strings.TrimSpace(strings.ToLower(profile))
	if p == "" || p == "off" || p == "none" {
		return "", nil
	}
	patterns, ok := DefaultProfiles[p]
	if !ok {
		return "", fmt.Errorf("unknown reflect-prune profile %q (supported: off, tier1, tier2)", profile)
	}
	return strings.Join(patterns, ","), nil
}
