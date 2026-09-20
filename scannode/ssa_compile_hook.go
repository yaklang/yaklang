//go:build !irify_exclude

package scannode

// ScanProject compiles through ssa_compile. That package cannot be imported by
// syntaxflow_scan (ssa_compile -> yakscript -> yak -> syntaxflow_scan), so the
// compile function is registered in ssa_compile init. Distyak is this same
// binary; without the hook, product scans fail with "compiler is not registered".
import _ "github.com/yaklang/yaklang/common/yak/ssa_compile"
