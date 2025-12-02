package yakvm

import (
	"errors"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

type SymbolTable struct {
	Verbose            string
	currentSymbolIndex int
	//idToValue  map[int]*Value

	parent   *SymbolTable
	children []*SymbolTable

	symbolToId map[string]int
	InitedId   map[int]struct{}
	index      int
	// 只有根表才有
	tableCount      int
	idToSymbolTable map[int]*SymbolTable
}

var symbolTableCount int32 = 0
var IdtoSymbolTable = new(sync.Map)

// func GetCurrentTableCount() int {
// 	return int(symbolTableCount)
// }

// func GetSymbolTableById(id int) *SymbolTable {
// 	tbl, ok := IdtoSymbolTable.Load(int32(id))
// 	if !ok {
// 		panic(fmt.Sprintf("cannot find symbol table[%v]", id))
// 	}
// 	return tbl.(*SymbolTable)
// }

//func (s *SymbolTable) Copy(lock bool) *SymbolTable {
//	if s.IsRoot {
//		panic("BUG: root symbol table cannot be copied! be careful!")
//	}
//	symbolToId := make(map[string]int, len(s.symbolToId))
//	for k, v := range s.symbolToId {
//		symbolToId[k] = v
//	}
//	symtbl := &SymbolTable{
//		Verbose:            s.Verbose,
//		IsRoot:             s.IsRoot,
//		symbolToId:         symbolToId,
//		parent:             s.parent,
//		children:           s.children,
//		currentSymbolIndex: s.currentSymbolIndex,
//	}
//	idToValue := make(map[int]*Value, len(s.idToValue))
//	var idToSymbolTable = s.idToSymbolTable
//	if lock {
//		idToSymbolTable = make(map[int]*SymbolTable, len(idToValue))
//	}
//	for k, v := range s.idToValue {
//		idToValue[k] = v
//		if lock {
//			idToSymbolTable[k] = symtbl
//		}
//	}
//	symtbl.idToValue = idToValue
//	symtbl.idToSymbolTable = idToSymbolTable
//	return symtbl
//}

//func (s *SymbolTable) FindSymbolTableBySymbolId(id int) (*SymbolTable, error) {
//	symbolMutex.Lock()
//	defer symbolMutex.Unlock()
//
//	current := s
//	for !current.IsRoot {
//		if current.idToSymbolTable != nil {
//			if tbl, ok := current.idToSymbolTable[id]; ok {
//				return tbl, nil
//			}
//		}
//
//		current = current.parent
//	}
//	t, ok := current.idToSymbolTable[id]
//	if !ok {
//		return nil, errors.New("BUG: cannot found symbol id[" + fmt.Sprint(id) + "] 's table")
//	}
//	return t, nil
//}

func NewSymbolTable() *SymbolTable {
	tbl := &SymbolTable{
		Verbose: "root",
		//IsRoot: true,
		symbolToId: make(map[string]int),
		//idToValue:       make(map[int]*Value),
		index:           1,
		tableCount:      1,
		idToSymbolTable: make(map[int]*SymbolTable),
		InitedId:        make(map[int]struct{}),
	}
	tbl.idToSymbolTable[1] = tbl
	return tbl
}

func (s *SymbolTable) IsNew() bool {
	return s.Verbose == "root" && (s.currentSymbolIndex == 0)
}
func (s *SymbolTable) GetTableCount() int {
	return s.tableCount
}
func (s *SymbolTable) SetIdIsInited(id int) {
	s.InitedId[id] = struct{}{}
}
func (s *SymbolTable) IdIsInited(id int) bool {
	_, ok := s.InitedId[id]
	return ok
}

func (s *SymbolTable) GetNameByVariableId(id int) (string, bool) {
	for name, i := range s.symbolToId {
		if i == id {
			return name, true
		}
	}
	//if s.IsRoot {
	//	return "", false
	//}
	return "", false
}

func (s *SymbolTable) GetSymbolByVariableName(name string) (int, bool) {
	if s == nil {
		return 0, false
	}
	i, ok := s.symbolToId[name]
	//if s.IsRoot {
	// 如果定义域是根节点
	//if ok {
	//	return i, true
	//} else {
	//	return -1, false
	//}
	//}
	if ok {
		return i, true
	}
	return s.parent.GetSymbolByVariableName(name)
}

func (s *SymbolTable) GetLocalSymbolByVariableName(name string) (int, bool) {
	if s == nil {
		return 0, false
	}
	i, ok := s.symbolToId[name]
	return i, ok
}

//func (s *SymbolTable) GetValueByVariableName(name string) (*Value, bool) {
//	id, ok := s.GetSymbolByVariableName(name)
//	if !ok {
//		return nil, false
//	}
//	return s.GetValueByVariableId(id)
//}

//func (s *SymbolTable) GetValueByVariableId(id int) (*Value, bool) {
//	i, ok := s.idToValue[id]
//	if s.IsRoot {
//		// 如果定义域是根节点
//		if ok {
//			return i, true
//		} else {
//			return nil, false
//		}
//	}
//	if ok {
//		return i, true
//	}
//	return s.parent.GetValueByVariableId(id)
//}

//func (s *SymbolTable) ForceSetSymbolValue(id int, val *Value) error {
//	symbolMutex.Lock()
//	defer symbolMutex.Unlock()
//
//	current := s
//	for !current.IsRoot {
//		_, ok := current.idToValue[id]
//		if !ok {
//			current = current.parent
//			continue
//		}
//		current.idToValue[id] = val
//		return nil
//	}
//	_, ok := current.idToValue[id]
//	if !ok {
//		return errors.New("cannot found symbol: " + fmt.Sprint(id))
//	}
//	current.idToValue[id] = val
//	return nil
//}

var (
	symbolMutex        = new(sync.Mutex)
	symbolTableMutex   = new(sync.Mutex)
	requireSymbolMutex = new(sync.Mutex)
)

func (s *SymbolTable) requireId(target *SymbolTable) int {
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
	return t.currentSymbolIndex
}

func (s *SymbolTable) NewSymbolWithReturn(name string) (int, error) {
	symbolMutex.Lock()
	defer symbolMutex.Unlock()

	_, ok := s.symbolToId[name]
	if ok && name != "_" && name != "err" {
		log.Warnf("assign new symbol by name[%v]: the name `%v` is existed before... re-defined variable!", name, name)
	}
	var id = s.requireId(s)
	s.symbolToId[name] = id
	return id, nil
}

func (s *SymbolTable) NewSymbolWithoutName() int {
	symbolMutex.Lock()
	defer symbolMutex.Unlock()

	var id = s.requireId(s)
	return id
}

func (s *SymbolTable) CreateSubSymbolTable(verbose ...string) *SymbolTable {
	sub := &SymbolTable{
		Verbose:    strings.Join(verbose, "/"),
		symbolToId: make(map[string]int),
		parent:     s,
		InitedId:   make(map[int]struct{}),
	}
	s.children = append(s.children, sub)
	root, err := s.GetRoot()
	if err != nil {
		panic(err)
	}
	symbolTableMutex.Lock()
	defer symbolTableMutex.Unlock()
	root.tableCount++
	root.idToSymbolTable[root.tableCount] = sub
	sub.index = root.tableCount

	return sub
}

func (s *SymbolTable) GetRoot() (*SymbolTable, error) {
	if s == nil {
		return nil, errors.New("BUG: symboltable is nil")
	}
	current := s
	for current.parent != nil {
		current = current.parent
	}
	if current.Verbose != "root" {
		return nil, errors.New("BUG: oldest symboltable is not root")
	}

	return current, nil
}

func (s *SymbolTable) MustRoot() *SymbolTable {
	root, err := s.GetRoot()
	if err != nil {
		panic(err)
	}
	return root
}

func (s *SymbolTable) GetTableIndex() int {
	return s.tableCount
}

func (s *SymbolTable) GetParentSymbolTable() *SymbolTable {
	return s.parent
}

func (s *SymbolTable) GetSymbolTableById(id int) *SymbolTable {
	root, err := s.GetRoot()
	if err != nil {
		panic(err)
	}
	// 添加锁保护，避免并发读写 idToSymbolTable map
	symbolTableMutex.Lock()
	defer symbolTableMutex.Unlock()
	return root.idToSymbolTable[id]
}
