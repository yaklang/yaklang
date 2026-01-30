# Critical Fix: Switch to Boehm GC for Memory Management

## Background
The current Pure Go memory management using `shadowStore` (global map) and `SetFinalizer` has two fatal flaws:
1.  **The Map Leak**: The global map holds strong references, preventing `SetFinalizer` from ever triggering.
2.  **The Stack Blindspot**: Go GC cannot scan the LLVM (C) stack, so temporary objects in loops are not collected.

## Solution
Switch to `libgc` (Boehm GC).
- Use `GC_MALLOC` to allocate shadow objects (direct C pointers).
- Use `GC_REGISTER_FINALIZER` to handle resource cleanup.
- Boehm GC can scan the C stack, solving the loop issue.

## Implementation Steps

### Step 1: Prepare C Stub (`common/yak/ssa2llvm/runtime/runtime_go/c_stub.c`)
Create a C stub to bridge the finalizer callback from C to Go.
```c
#include <gc.h>
#include "_cgo_export.h" 

void yak_finalizer_proxy(void* obj, void* client_data) {
    yak_internal_release_shadow(obj);
}
```

### Step 2: Refactor Runtime (`common/yak/ssa2llvm/runtime/runtime_go/yak_lib.go`)
- Remove `shadowStore` and locks.
- Enable CGO linking with `-lgc`.
- Update `yak_runtime_new_shadow`:
    - Allocate using `C.GC_MALLOC(8)`.
    - Register `yak_finalizer_proxy`.
    - Return the raw C pointer.
- Add `yak_internal_release_shadow`:
    - Exported for C.
    - Releases the underlying Go Handle.

### Step 3: Adjust Compiler (`common/yak/ssa2llvm/compiler/ops_memory.go` & `ops.go`)
- Change `YakObject` type in LLVM IR from `i64` (ID) to `ptr` (Opaque Pointer).
- Ensure `getObject` and related ops use pointer semantics.
- Fix `compileMakeGeneric` and `compileMakeStruct` to use correct types.

### Step 4: Verification
- Create `TestInterop_LoopGC` to verify memory stability in tight loops.
