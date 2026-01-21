# SSA to LLVM Compiler - Project Structure

## Directory Layout

```
common/yak/ssa2llvm/
├── cmd/
│   └── yakssa.go              # CLI entry point (compile, run commands)
├── compiler/
│   ├── compiler.go            # Core compiler orchestration
│   ├── ops.go                 # Arithmetic operations (add, sub, mul, div, mod)
│   ├── ops_control.go         # Control flow (if, jump, return, phi)
│   ├── ops_call.go            # Function calls
│   ├── ops_memory.go          # Memory operations (TODO: Phase 2)
│   ├── arithmetic_test.go     # Unit tests for arithmetic
│   ├── call_test.go           # Unit tests for calls
│   └── control_test.go        # Unit tests for control flow
├── types/
│   ├── converter.go           # Type conversion (TODO: Phase 2)
│   └── layout.go              # Memory layouts (TODO: Phase 2)
├── runtime/
│   ├── runtime.c              # C runtime (TODO: Phase 2)
│   └── bindings.go            # LLVM bindings for runtime (TODO: Phase 2)
├── tests/
│   ├── basic_test.go          # Basic arithmetic tests
│   ├── call_test.go           # Function call tests
│   ├── cfg_test.go            # Control flow tests
│   ├── multilang_test.go      # Multi-language tests
│   ├── jit_test.go            # JIT execution tests
│   ├── runner_test.go         # Integration test runner
│   ├── test_utils.go          # Test utilities
│   ├── testdata_compile_test.go  # Testdata compilation test
│   └── testdata/              # Multi-language example files
│       ├── example.yak
│       ├── example.go
│       ├── example.py
│       ├── example.js
│       ├── example.ts
│       ├── example.java
│       ├── example.php
│       └── example.c
└── STATUS.md                  # Implementation status documentation
```

## Current Status (Phase 1)

### ✅ Completed
- Basic arithmetic (+, -, *, /, %)
- Comparisons (>, <, >=, <=, ==, !=)
- Control flow (if-else, loops, break)
- Functions (simple, nested, recursive, multi-param)
- SSA constructs (Phi nodes, basic blocks)
- Multi-language support (Yak, Go, Python, JS, Java, PHP, C, TypeScript)
- **All 15 tests passing (100%)**

### ⏳ Phase 2 (TODO)
- Type system (float, string, bool, arrays, structs, maps)
- Memory operations (Make, MemberCall, field access, indexing)
- C runtime (map ops, string ops, memory management, exceptions)
- LLVM optimization passes

## CLI Usage

### Compile to LLVM IR
```bash
yak ssa compile example.yak -o output.ll --print-ir
```

### Run via JIT
```bash
yak ssa run example.yak
```

### Options
- `--language, -l`: Source language (yak, go, python, javascript, etc.)
- `--output, -o`: Output file path
- `--print-ir`: Print generated LLVM IR
- `--function, -f`: Entry function name (default: check)
- `--verify`: Verify LLVM module (default: true)

## Test Commands

```bash
# Run all tests
go test ./common/yak/ssa2llvm/tests/...

# Run specific test categories
go test ./common/yak/ssa2llvm/tests -run TestBasic
go test ./common/yak/ssa2llvm/tests -run TestCall
go test ./common/yak/ssa2llvm/tests -run TestCFG
go test ./common/yak/ssa2llvm/tests -run TestLang

# Run testdata compilation tests
go test ./common/yak/ssa2llvm/tests -run TestCompileTestdata
```

## Architecture

### Compilation Pipeline

1. **Parse**: Source → SSA (via ssaapi.Parse)
2. **Setup**: Create LLVM Module, Functions, BasicBlocks
3. **Compile**: 
   - Pass 1: Create Phi nodes
   - Pass 2: Compile instructions
   - Pass 3: Add terminators
   - Pass 4: Resolve Phi incoming values
4. **Verify**: Check LLVM module validity
5. **Execute**: JIT or AOT compilation

### Key Design Decisions

1. **All values are i64**: Phase 1 treats everything as 64-bit integers
2. **Terminator inference**: Since YakSSA If/Jump may not be in Values map, we infer from BasicBlock successors
3. **Phi resolution**: Handle YakSSA's Undefined edges by filtering and fallback strategies
4. **Resource management**: ExecutionEngine owns Module (avoid double-free)

## Example Code

```go
package main

import (
    "context"
    "github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
    "github.com/yaklang/yaklang/common/yak/ssaapi"
    "tinygo.org/x/go-llvm"
)

func main() {
    llvm.InitializeNativeTarget()
    llvm.InitializeNativeAsmPrinter()
    
    code := `check = () => { return 42 }`
    prog, _ := ssaapi.Parse(code)
    
    c := compiler.NewCompiler(context.Background(), prog.Program)
    c.Compile()
    
    engine, _ := llvm.NewExecutionEngine(c.Mod)
    defer engine.Dispose()
    
    fn := c.Mod.NamedFunction("check")
    result := engine.RunFunction(fn, []llvm.GenericValue{})
    
    println(result.Int(true)) // Output: 42
}
```

## Contributing

See [STATUS.md](STATUS.md) for detailed implementation status and future roadmap.
