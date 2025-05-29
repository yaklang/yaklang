package reducer

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils/linktable"
	"sync"
)

type ReduceFunction func([]string) string

// Reducer is a struct that handles the reduction of memory size
type Reducer struct {
	reduceHandler ReduceFunction
	reduceLimit   int
	lock          sync.Mutex
	data          []*linktable.LinkTable[string]
	allData       []*linktable.LinkTable[string]
}

func NewReducer(reduceLimit int, handle ReduceFunction) *Reducer {
	return &Reducer{
		reduceLimit:   reduceLimit,
		reduceHandler: handle,
		data:          make([]*linktable.LinkTable[string], 0),
		allData:       make([]*linktable.LinkTable[string], 0),
	}
}

func (r *Reducer) SetReduceFunction(handle ReduceFunction) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.reduceHandler = handle
}

func (r *Reducer) Push(data string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.allData = append(r.allData, linktable.NewUnlimitedLinkTable[string](data))
	r.data = append(r.data, linktable.NewUnlimitedLinkTable[string](data))
	dataCount := len(r.data)
	if r.reduceLimit > 0 && dataCount-r.reduceLimit > 0 {
		endIdx := dataCount - r.reduceLimit
		if len(r.data) > endIdx {
			r.reduce(endIdx)
		}
	}
}

func (r *Reducer) _getBeforeData(index int) (beforeData []string) {
	for i := 0; i <= index; i++ {
		beforeData = append(beforeData, r.data[i].Value())
	}
	return
}

func (r *Reducer) Dump() string {
	return spew.Sdump(r.GetData())
}

func (r *Reducer) GetData() []string {
	r.lock.Lock()
	defer r.lock.Unlock()
	data := make([]string, 0)
	for _, datum := range r.data {
		data = append(data, datum.Value())
	}
	return data
}

func (r *Reducer) DumpAll() string {
	r.lock.Lock()
	defer r.lock.Unlock()
	return spew.Sdump(r.allData)
}

func (r *Reducer) reduce(beforeId int) {
	if beforeId <= 0 || beforeId >= len(r.data) || r.reduceHandler == nil {
		return
	}
	beforeData := r._getBeforeData(beforeId)
	if len(beforeData) == 0 || r.reduceHandler == nil {
		return
	}
	newReduceData := r.reduceHandler(beforeData)
	if newReduceData != "" {
		lt := r.data[beforeId]
		lt.Push(newReduceData)
	}
	r.data = r.data[beforeId:]
}
