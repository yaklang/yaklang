package ssa

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// irTypeJSONBufPool is process-wide on purpose. writeTo copies the JSON out
// with Buffer.String before Put, so concurrent programs never share a buffer
// that is still being written. A pool per program would only add memory.
var irTypeJSONBufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

const (
	typePersistenceGeneric = iota
	typePersistenceNamed
	typePersistenceBasic
	typePersistenceBlueprint
)

// These are exactly the fields persisted by type2IrType. Capture them while
// the compiler owns the type graph; encoding workers never call Type methods.
type typePersistenceSnapshot struct {
	id              int64
	kind, extraKind int
	blueprintKind   string
	shape           int
	text, name      string
	fullNames       []string
	parents, ifaces []int64
	container       int64
}

func snapshotTypePersistence(typ Type) typePersistenceSnapshot {
	s := typePersistenceSnapshot{id: typ.GetId(), kind: int(typ.GetTypeKind()), text: typ.String(), fullNames: typ.GetFullTypeNames()}
	switch t := typ.(type) {
	case *FunctionType:
		s.shape, s.name = typePersistenceNamed, t.Name
	case *ObjectType:
		s.shape, s.name = typePersistenceNamed, t.Name
	case *BasicType:
		s.shape, s.name, s.extraKind = typePersistenceBasic, t.name, int(t.Kind)
	case *Blueprint:
		s.shape, s.name, s.blueprintKind = typePersistenceBlueprint, t.Name, string(t.Kind)
		for _, p := range t.ParentBlueprints {
			s.parents = append(s.parents, p.GetId())
		}
		for _, p := range t.InterfaceBlueprints {
			s.ifaces = append(s.ifaces, p.GetId())
		}
		s.container = -1
		if c := t.Container(); !utils.IsNil(c) {
			s.container = c.GetId()
		}
	}
	return s
}

func (s typePersistenceSnapshot) fingerprint() [sha256.Size]byte {
	h := sha256.New()
	var number [8]byte
	integer := func(v int64) { binary.LittleEndian.PutUint64(number[:], uint64(v)); h.Write(number[:]) }
	str := func(v string) { integer(int64(len(v))); h.Write(utils.UnsafeStringToBytes(v)) }
	integer(int64(s.shape))
	integer(int64(s.kind))
	integer(int64(s.extraKind))
	str(s.text)
	str(s.name)
	str(s.blueprintKind)
	if s.fullNames == nil {
		integer(-1)
	} else {
		integer(int64(len(s.fullNames)))
	}
	for _, name := range s.fullNames {
		str(name)
	}
	integer(int64(len(s.parents)))
	for _, id := range s.parents {
		integer(id)
	}
	integer(int64(len(s.ifaces)))
	for _, id := range s.ifaces {
		integer(id)
	}
	integer(s.container)
	var result [sha256.Size]byte
	h.Sum(result[:0])
	return result
}

func (s typePersistenceSnapshot) writeTo(ir *ssadb.IrType) {
	param := map[string]any{"fullTypeName": s.fullNames}
	if s.shape != typePersistenceGeneric {
		param["name"] = s.name
	}
	if s.shape == typePersistenceBasic {
		param["kind"] = s.extraKind
	}
	if s.shape == typePersistenceBlueprint {
		param["kind"] = s.blueprintKind
		param["parentBlueprints"], param["interfaceBlueprints"] = s.parents, s.ifaces
		param["container"] = s.container
	}
	buf := irTypeJSONBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	if err := json.NewEncoder(buf).Encode(param); err != nil {
		log.Errorf("SaveTypeToDB: %v", err)
	}
	ir.TypeId, ir.Kind, ir.String = uint64(s.id), s.kind, s.text
	ir.ExtraInformation = strings.TrimSuffix(buf.String(), "\n")
	buf.Reset()
	irTypeJSONBufPool.Put(buf)
}

func marshalTypeSnapshots(program string, snapshots []typePersistenceSnapshot, concurrency int) []*ssadb.IrType {
	rows := make([]*ssadb.IrType, len(snapshots))
	if concurrency <= 0 {
		concurrency = ssaconfig.DefaultCPUConcurrency()
	}
	// Small updates do not benefit from launching workers. Large updates use
	// the configured CPU budget, with no fixed eight-core cap. Only this save
	// batch is materialized; SQLite writes remain on the caller goroutine.
	workers := min(concurrency, (len(snapshots)+63)/64)
	encode := func(i int) {
		rows[i] = ssadb.EmptyIrType(program, uint64(snapshots[i].id))
		snapshots[i].writeTo(rows[i])
	}
	if workers <= 1 {
		for i := range snapshots {
			encode(i)
		}
		return rows
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1) - 1)
				if i >= len(snapshots) {
					return
				}
				encode(i)
			}
		}()
	}
	wg.Wait()
	return rows
}
