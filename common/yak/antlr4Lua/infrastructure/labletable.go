package infrastructure

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"yaklang.io/yaklang/common/log"
)

type LabelTable struct {
	Verbose            string
	currentSymbolIndex int
	//idToValue  map[int]*Value

	parent   *LabelTable
	children []*LabelTable

	symbolToId   map[string]int
	idToJmpIndex map[int]int
}

var labelTableCount int32 = 0
var IdtoLabelTable = new(sync.Map)

func GetCurrentTableCount() int {
	return int(labelTableCount)
}

func GetSymbolTableById(id int) *LabelTable {
	tbl, ok := IdtoLabelTable.Load(int32(id))
	if !ok {
		panic(fmt.Sprintf("cannot find symbol table[%d]", id))
	}
	return tbl.(*LabelTable)
}

var (
	labelMutex = new(sync.Mutex)
)

func NewLabelTable() *LabelTable {
	tbl := &LabelTable{
		Verbose:      "root",
		symbolToId:   make(map[string]int),
		idToJmpIndex: make(map[int]int),
	}
	atomic.AddInt32(&labelTableCount, 1)
	IdtoLabelTable.Store(atomic.LoadInt32(&labelTableCount), tbl)
	return tbl
}

func (s *LabelTable) GetNameByVariableId(id int) (string, bool) {
	for name, i := range s.symbolToId {
		if i == id {
			return name, true
		}
	}

	return "", false
}

func (s *LabelTable) GetSymbolByVariableName(name string) (int, bool) {
	if s == nil {
		return 0, false
	}
	i, ok := s.symbolToId[name]

	if ok {
		return i, true
	}
	return s.parent.GetSymbolByVariableName(name)
}

func (s *LabelTable) GetJmpIndexByVariableId(id int) (int, bool) {
	for i, codeIndex := range s.idToJmpIndex {
		if i == id {
			return codeIndex, true
		}
	}
	return -1, false
}

var requireSymbolMutex = new(sync.Mutex)

func (s *LabelTable) requireId(target *LabelTable) int {
	requireSymbolMutex.Lock()
	defer requireSymbolMutex.Unlock()

	var t = s
	if t == nil {
		log.Errorf("yak symbol table error, no root symbol table")
		return -1
	}

	for t.parent != nil {
		t = t.parent
	}

	t.currentSymbolIndex++
	//t.idToSymbolTable[t.currentSymbolIndex] = target
	return t.currentSymbolIndex
}

func (s *LabelTable) NewSymbolWithReturn(name string, codeIndex int) (int, error) {
	labelMutex.Lock()
	defer labelMutex.Unlock()

	_, ok := s.symbolToId[name]
	if ok && name != "_" {
		log.Warnf("assign new symbol by name[%v]: the name `%v` is existed before... re-defined variable!", name, name)
	}
	var id = s.requireId(s)
	s.symbolToId[name] = id
	s.idToJmpIndex[id] = codeIndex
	return id, nil
}

func (s *LabelTable) CreateSubSymbolTable(verbose ...string) *LabelTable {
	sub := &LabelTable{
		Verbose:      strings.Join(verbose, "/"),
		symbolToId:   make(map[string]int),
		idToJmpIndex: make(map[int]int),
		parent:       s,
	}
	s.children = append(s.children, sub)

	atomic.AddInt32(&labelTableCount, 1)
	IdtoLabelTable.Store(atomic.LoadInt32(&labelTableCount), sub)
	return sub
}
func (s *LabelTable) GetParentSymbolTable() *LabelTable {
	return s.parent
}
