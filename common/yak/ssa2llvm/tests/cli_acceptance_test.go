package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/profile"
)

func TestSSA2LLVMCLICompileAndRunNativeArtifact(t *testing.T) {
	source := writeYakSourceFile(t, `
check = () => {
	println("native ok")
	return 42
}
`)
	bin := filepath.Join(t.TempDir(), "native.bin")

	compile := runSSA2LLVMCLI(t, "compile", source, "-o", bin, "-f", "check")
	require.Equal(t, 0, compile.ExitCode, compile.Output)

	info, err := os.Stat(bin)
	require.NoError(t, err)
	require.False(t, info.IsDir())

	run := runProcess(t, bin, nil)
	require.Equal(t, 42, run.ExitCode, run.Output)
	require.Contains(t, run.Output, "native ok")
}

func TestSSA2LLVMCLICompileAndRunCallretArtifact(t *testing.T) {
	source := writeYakSourceFile(t, `
leaf = () => { return 7 }
mid = () => { return leaf() + 8 }
check = () => {
	println("callret ok")
	return mid() + leaf()
}
`)
	bin := filepath.Join(t.TempDir(), "callret.bin")

	compile := runSSA2LLVMCLI(t, "compile", source, "-o", bin, "-f", "check", "--obf", "callret")
	require.Equal(t, 0, compile.ExitCode, compile.Output)

	run := runProcess(t, bin, nil)
	require.Equal(t, 22, run.ExitCode, run.Output)
	require.Contains(t, run.Output, "callret ok")
}

func TestSSA2LLVMCLICompileAndRunProfileLiteArtifact(t *testing.T) {
	source := writeYakSourceFile(t, `
leaf = () => { return 20 }
check = () => {
	println("profile lite ok")
	return leaf() + 22
}
`)
	bin := filepath.Join(t.TempDir(), "profile-lite.bin")

	compile := runSSA2LLVMCLI(t, "compile", source, "-o", bin, "-f", "check", "--profile", "resilience-lite")
	require.Equal(t, 0, compile.ExitCode, compile.Output)

	run := runProcess(t, bin, nil)
	require.Equal(t, 42, run.ExitCode, run.Output)
	require.Contains(t, run.Output, "profile lite ok")
}

func TestSSA2LLVMCLICompileEmitLLVMCallretArtifact(t *testing.T) {
	source := writeYakSourceFile(t, `
leaf = () => { return 7 }
mid = () => { return leaf() + 8 }
check = () => {
	return mid() + leaf()
}
`)
	ll := filepath.Join(t.TempDir(), "callret.ll")

	compile := runSSA2LLVMCLI(t, "compile", source, "-o", ll, "-f", "check", "--emit-llvm", "--obf", "callret")
	require.Equal(t, 0, compile.ExitCode, compile.Output)

	data, err := os.ReadFile(ll)
	require.NoError(t, err)
	text := string(data)
	require.Contains(t, text, "obf_vs_sp")
	require.Contains(t, text, "obf_cs_sp")
}

func TestSSA2LLVMCLIRunSubcommandMatchesRealUsage(t *testing.T) {
	source := writeYakSourceFile(t, `
check = () => {
	println("run command ok")
	return 42
}
`)

	run := runSSA2LLVMCLI(t, "run", source, "-f", "check")
	require.Equal(t, 42, run.ExitCode, run.Output)
	require.Contains(t, run.Output, "run command ok")
}

// ---------------------------------------------------------------------------
// Profile-file, virtualize, profile, and flag acceptance tests
// ---------------------------------------------------------------------------

func TestSSA2LLVMCLIProfileCallretCompileAndRun(t *testing.T) {
	source := writeYakSourceFile(t, `
leaf = () => { return 7 }
mid = () => { return leaf() + 8 }
	check = () => {
	println("profile file callret ok")
	return mid() + leaf()
}
`)
	profPath := writeProfileFile(t, &profile.Profile{
		SelectionSeed: 42,
		Obfuscators: []profile.ObfEntry{
			{Name: "callret", Category: profile.CategoryCallflow, Selector: profile.Selector{AllowEntry: true}},
		},
	})
	bin := filepath.Join(t.TempDir(), "profile-file-callret.bin")

	compile := runSSA2LLVMCLI(t, "compile", source, "-o", bin, "-f", "check",
		"--profile", profPath)
	require.Equal(t, 0, compile.ExitCode, compile.Output)

	run := runProcess(t, bin, nil)
	require.Equal(t, 22, run.ExitCode, run.Output)
	require.Contains(t, run.Output, "profile file callret ok")
}

func TestSSA2LLVMCLIProfileEmitLLVM(t *testing.T) {
	source := writeYakSourceFile(t, `
leaf = () => { return 7 }
mid = () => { return leaf() + 8 }
check = () => { return mid() + leaf() }
`)
	profPath := writeProfileFile(t, &profile.Profile{
		SelectionSeed: 42,
		Obfuscators: []profile.ObfEntry{
			{Name: "callret", Category: profile.CategoryCallflow, Selector: profile.Selector{AllowEntry: true}},
		},
	})
	ll := filepath.Join(t.TempDir(), "profile-file.ll")

	compile := runSSA2LLVMCLI(t, "compile", source, "-o", ll, "-f", "check",
		"--emit-llvm", "--profile", profPath)
	require.Equal(t, 0, compile.ExitCode, compile.Output)

	data, err := os.ReadFile(ll)
	require.NoError(t, err)
	text := string(data)
	require.Contains(t, text, "obf_vs_sp", "callret virtual stack should appear in IR")
	require.Contains(t, text, "obf_cs_sp", "callret call stack should appear in IR")
}

func TestSSA2LLVMCLIVirtualizeEmitLLVM(t *testing.T) {
	source := writeYakSourceFile(t, `
compute = () => {
	a = 10
	b = 20
	c = a + b
	return c * 2
}
check = () => { return compute() }
`)
	ll := filepath.Join(t.TempDir(), "virt.ll")

	compile := runSSA2LLVMCLI(t, "compile", source, "-o", ll, "-f", "check",
		"--emit-llvm", "--obf", "virtualize")
	require.Equal(t, 0, compile.ExitCode, compile.Output)

	data, err := os.ReadFile(ll)
	require.NoError(t, err)
	text := string(data)
	require.Contains(t, text, "yak_runtime_invoke_vm",
		"virtualized stub should call the VM runtime")
	require.Contains(t, text, "yak_virtualize_payload_",
		"virtualized stub should embed payload constants")
}

func TestSSA2LLVMCLIVirtualizeCallretEmitLLVM(t *testing.T) {
	source := writeYakSourceFile(t, `
compute = () => {
	a = 10
	b = 20
	c = a + b
	return c * 2
}
check = () => { return compute() }
`)
	ll := filepath.Join(t.TempDir(), "virt-callret.ll")

	// When both virtualize and callret are active, virtualize claims
	// lowerable functions; callret skips body-replaced functions gracefully.
	compile := runSSA2LLVMCLI(t, "compile", source, "-o", ll, "-f", "check",
		"--emit-llvm", "--obf", "virtualize", "--obf", "callret")
	require.Equal(t, 0, compile.ExitCode, compile.Output)

	data, err := os.ReadFile(ll)
	require.NoError(t, err)
	text := string(data)
	require.Contains(t, text, "yak_runtime_invoke_vm",
		"virtualize wrapper present")
}

func TestSSA2LLVMCLIProfileHybridCompileAndRun(t *testing.T) {
	source := writeYakSourceFile(t, `
compute = () => {
	a = 10
	b = 20
	c = a + b
	return c * 2
}
check = () => {
	println("profile hybrid ok")
	return compute()
}
`)
	bin := filepath.Join(t.TempDir(), "profile-hybrid.bin")

	compile := runSSA2LLVMCLI(t, "compile", source, "-o", bin, "-f", "check",
		"--profile", "resilience-hybrid")
	require.Equal(t, 0, compile.ExitCode, compile.Output)

	run := runProcess(t, bin, nil)
	require.Equal(t, 60, run.ExitCode, run.Output)
	require.Contains(t, run.Output, "profile hybrid ok")
}

func TestSSA2LLVMCLIProfileMaxCompileAndRun(t *testing.T) {
	source := writeYakSourceFile(t, `
compute = () => {
	a = 10
	b = 20
	c = a + b
	return c * 2
}
check = () => {
	println("profile max ok")
	return compute()
}
`)
	bin := filepath.Join(t.TempDir(), "profile-max.bin")

	compile := runSSA2LLVMCLI(t, "compile", source, "-o", bin, "-f", "check",
		"--profile", "resilience-max")
	require.Equal(t, 0, compile.ExitCode, compile.Output)

	run := runProcess(t, bin, nil)
	require.Equal(t, 60, run.ExitCode, run.Output)
	require.Contains(t, run.Output, "profile max ok")
}

func TestSSA2LLVMCLIObfuscatorsListsVirtualize(t *testing.T) {
	result := runSSA2LLVMCLI(t, "obfuscators")
	require.Equal(t, 0, result.ExitCode, result.Output)
	require.Contains(t, result.Output, "virtualize",
		"obfuscators subcommand should list virtualize")
	require.Contains(t, result.Output, "callret")
	require.Contains(t, result.Output, "addsub")
	require.Contains(t, result.Output, "xor")
}

func TestSSA2LLVMCLIRemovedProtectFlags(t *testing.T) {
	// Verify --protect-func and --protect-all are NOT in the help text.
	help := runSSA2LLVMCLI(t, "compile", "--help")
	require.NotContains(t, help.Output, "protect-func",
		"--protect-func should not appear in CLI help")
	require.NotContains(t, help.Output, "protect-all",
		"--protect-all should not appear in CLI help")

	// Verify --profile is the only profile/config flag exposed.
	require.Contains(t, help.Output, "profile",
		"--profile should appear in CLI help")
	require.NotContains(t, help.Output, "obf-policy",
		"--obf-policy should not appear in CLI help")
	require.Contains(t, help.Output, "built-in profile name or load a profile JSON file",
		"--profile help should describe both built-in names and file paths")
}

func TestSSA2LLVMCLIProfileBuiltinNameWinsOverLocalFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "input.yak")
	require.NoError(t, os.WriteFile(source, []byte(`
leaf = () => { return 20 }
check = () => {
	println("builtin profile wins")
	return leaf() + 22
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "resilience-lite"), []byte(`not-json`), 0o644))

	bin := filepath.Join(dir, "builtin-priority.bin")
	compile := runSSA2LLVMCLIInDir(t, dir, "compile", source, "-o", bin, "-f", "check", "--profile", "resilience-lite")
	require.Equal(t, 0, compile.ExitCode, compile.Output)

	run := runProcess(t, bin, nil)
	require.Equal(t, 42, run.ExitCode, run.Output)
	require.Contains(t, run.Output, "builtin profile wins")
}

// writeProfileFile serialises a profile to a temp JSON file and returns its path.
func writeProfileFile(t *testing.T, prof *profile.Profile) string {
	t.Helper()
	data, err := json.Marshal(prof)
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "profile.json")
	require.NoError(t, os.WriteFile(p, data, 0o644))
	return p
}
