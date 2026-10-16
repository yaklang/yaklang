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
		s.shape, s.name, s.extraKind = typePersistenceBasic, t.GetName(), int(t.Kind)
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

func marshalTypeSnapshots(program string, snapshots []typePersistenceSnapshot, fingerprints [][sha256.Size]byte, concurrency int) []*ssadb.IrType {
	rows := make([]*ssadb.IrType, len(snapshots))
	if len(snapshots) == 0 {
		return rows
	}
	if len(fingerprints) != len(snapshots) {
		fingerprints = make([][sha256.Size]byte, len(snapshots))
		for i := range snapshots {
			fingerprints[i] = snapshots[i].fingerprint()
		}
	}
	// Same persisted payload can belong to different Type IDs. Encode it once
	// and copy the immutable strings. Do not alias the row structs or change IDs.
	first := make(map[[sha256.Size]byte]int, len(snapshots))
	unique := make([]int, 0, len(snapshots))
	for i, fp := range fingerprints {
		if _, ok := first[fp]; ok {
			continue
		}
		first[fp] = i
		unique = append(unique, i)
	}
	if concurrency <= 0 {
		concurrency = ssaconfig.DefaultCPUConcurrency()
	}
	workers := min(concurrency, (len(unique)+63)/64)
	encode := func(i int) {
		rows[i] = ssadb.EmptyIrType(program, uint64(snapshots[i].id))
		snapshots[i].writeTo(rows[i])
	}
	if workers <= 1 {
		for _, i := range unique {
			encode(i)
		}
	} else {
		var next atomic.Int64
		var wg sync.WaitGroup
		wg.Add(workers)
		for range workers {
			go func() {
				defer wg.Done()
				for {
					n := int(next.Add(1) - 1)
					if n >= len(unique) {
						return
					}
					encode(unique[n])
				}
			}()
		}
		wg.Wait()
	}
	for i, fp := range fingerprints {
		src := first[fp]
		if src == i {
			continue
		}
		encoded := rows[src]
		row := ssadb.EmptyIrType(program, uint64(snapshots[i].id))
		row.Kind, row.String, row.ExtraInformation = encoded.Kind, encoded.String, encoded.ExtraInformation
		rows[i] = row
	}
	return rows
}
