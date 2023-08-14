package surigen

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils/regen"
	"math"
	"math/rand"
)

type Modifier interface {
	Modify(payload *ByteMap) error
}

var ErrOverFlow = errors.New("bytemap too small")

// ContentModifier define a method to modfiy content
// Content should be put at [Offset, Offset + Range]
// If Relative is true, Offset should be relative former one,
// Negetive value of Offset is used as the reverse order (can't use together with Relative and Rand).
type ContentModifier struct {
	NoCase   bool
	Offset   int
	Range    int
	Relative bool
	Filter   func(free []int, payload *ByteMap, cm *ContentModifier) []int
	Content  []byte
}

func (m *ContentModifier) Modify(payload *ByteMap) error {
	begin := m.Offset
	if m.Relative {
		begin += payload.lastPos + payload.lastLen
	}
	begin = (begin + payload.Size()) % payload.Size()

	var end int
	if m.Range == math.MaxInt {
		// in case of overflow
		end = math.MaxInt
	} else {
		end = begin + m.Range + len(m.Content)
	}

	allfree := payload.FindFreeRange(len(m.Content), begin, end)
	if m.Filter != nil {
		allfree = m.Filter(allfree, payload, m)
	}
	if len(allfree) == 0 {
		return ErrOverFlow
	}

	if m.NoCase {
		payload.Fill(allfree[rand.Intn(len(allfree))], nocaseFilter(m.Content))
	} else {
		payload.Fill(allfree[rand.Intn(len(allfree))], m.Content)
	}
	return nil
}

type RegexpModifier struct {
	Gen      regen.Generator
	Relative bool
}

func (m *RegexpModifier) Modify(payload *ByteMap) {

}
