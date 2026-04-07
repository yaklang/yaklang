package runtime

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/encode"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/executor"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/pir"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/seed"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

type HostBinding struct {
	Dispatch abi.FuncID
	Symbol   string
}

var regionCache sync.Map // map[string]*pir.Region

var defaultBindings = map[string]HostBinding{
	"println":                {Dispatch: abi.IDPrintln},
	"print":                  {Dispatch: abi.IDPrint},
	"printf":                 {Dispatch: abi.IDPrintf},
	"append":                 {Dispatch: abi.IDAppend},
	"yakit.Info":             {Dispatch: abi.IDYakitInfo},
	"yakit.Warn":             {Dispatch: abi.IDYakitWarn},
	"yakit.Debug":            {Dispatch: abi.IDYakitDebug},
	"yakit.Error":            {Dispatch: abi.IDYakitError},
	"sync.NewWaitGroup":      {Dispatch: abi.IDSyncNewWaitGroup},
	"sync.NewSizedWaitGroup": {Dispatch: abi.IDSyncNewSizedWaitGroup},
	"sync.NewLock":           {Dispatch: abi.IDSyncNewLock},
	"sync.NewMutex":          {Dispatch: abi.IDSyncNewMutex},
	"sync.NewRWMutex":        {Dispatch: abi.IDSyncNewRWMutex},
	"sync.NewMap":            {Dispatch: abi.IDSyncNewMap},
	"sync.NewOnce":           {Dispatch: abi.IDSyncNewOnce},
	"sync.NewPool":           {Dispatch: abi.IDSyncNewPool},
	"sync.NewCond":           {Dispatch: abi.IDSyncNewCond},
	"poc.timeout":            {Dispatch: abi.IDPocTimeout},
	"poc.Get":                {Dispatch: abi.IDPocGet},
	"poc.GetHTTPPacketBody":  {Dispatch: abi.IDPocGetHTTPPacketBody},
	"os.Getenv":              {Dispatch: abi.IDOsGetenv},
}

func Execute(blobHex, seedHex, funcName, hostBindingSpec string, args []int64,
	dispatchFn func(abi.FuncID, []uint64) int64,
	symbolFn func(string, []int64) (int64, error),
) (int64, error) {
	region, err := decodeRegion(blobHex, seedHex)
	if err != nil {
		return 0, err
	}
	handler := hostCallHandler(region, parseHostBindingSpec(hostBindingSpec), dispatchFn, symbolFn)
	result, err := executor.ExecuteRegion(region, funcName, args, handler)
	if err != nil {
		return 0, err
	}
	return result.Value, nil
}

func decodeRegion(blobHex, seedHex string) (*pir.Region, error) {
	key := blobHex + "|" + seedHex
	if cached, ok := regionCache.Load(key); ok {
		if region, ok := cached.(*pir.Region); ok && region != nil {
			return region, nil
		}
	}

	blob, err := hex.DecodeString(blobHex)
	if err != nil {
		return nil, fmt.Errorf("vm runtime: decode blob hex: %w", err)
	}
	seedBytes, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, fmt.Errorf("vm runtime: decode seed hex: %w", err)
	}
	if len(seedBytes) != 32 {
		return nil, fmt.Errorf("vm runtime: invalid seed length %d", len(seedBytes))
	}
	var raw [32]byte
	copy(raw[:], seedBytes)

	region, err := encode.Decode(blob, seed.FromBytes(raw))
	if err != nil {
		return nil, err
	}
	regionCache.Store(key, region)
	return region, nil
}

func parseHostBindingSpec(spec string) map[string]HostBinding {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	out := make(map[string]HostBinding)
	for _, line := range strings.Split(spec, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, raw, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		raw = strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(raw, "D:"):
			value, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(raw, "D:")), 10, 64)
			if err != nil {
				continue
			}
			out[strings.TrimSpace(name)] = HostBinding{Dispatch: abi.FuncID(value)}
		case strings.HasPrefix(raw, "S:"):
			symbol := strings.TrimSpace(strings.TrimPrefix(raw, "S:"))
			if symbol == "" {
				continue
			}
			out[strings.TrimSpace(name)] = HostBinding{Symbol: symbol}
		}
	}
	return out
}

func hostCallHandler(region *pir.Region, overrides map[string]HostBinding,
	dispatchFn func(abi.FuncID, []uint64) int64,
	symbolFn func(string, []int64) (int64, error),
) executor.HostCallHandler {
	if region == nil || len(region.HostSymbols) == 0 {
		return nil
	}
	return func(callee int64, args []int64) (int64, error) {
		index := int(callee)
		if index < 0 || index >= len(region.HostSymbols) {
			return 0, fmt.Errorf("vm runtime: host symbol index %d out of range", index)
		}
		name := region.HostSymbols[index]
		binding, ok := defaultBindings[name]
		if !ok && overrides != nil {
			binding, ok = overrides[name]
		}
		if !ok {
			return 0, fmt.Errorf("vm runtime: unsupported host-call %q", name)
		}
		if binding.Symbol != "" {
			return symbolFn(binding.Symbol, args)
		}
		rawArgs := make([]uint64, len(args))
		for i, arg := range args {
			rawArgs[i] = uint64(arg)
		}
		return dispatchFn(binding.Dispatch, rawArgs), nil
	}
}
