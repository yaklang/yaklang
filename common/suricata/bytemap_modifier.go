package suricata

import (
	"github.com/dlclark/regexp2"
	"github.com/pkg/errors"
	"math"
	"math/rand"
)

type ByteMapModifier interface {
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
	Generator *PCREGenerator
}

func (m *RegexpModifier) Modify(payload *ByteMap) error {
	content := m.Generator.Generate()

	begin := 0
	end := math.MaxInt

	if m.Generator.startsWith {
		begin = 0
		end = len(content)
	} else if m.Generator.relative {
		begin = payload.lastPos + payload.lastLen
		end = begin + len(content)
	}

	allfree := payload.FindFreeRange(len(content), begin, end)
	if len(allfree) == 0 {
		return ErrOverFlow
	}

	if m.Generator.opts&regexp2.IgnoreCase != 0 {
		payload.Fill(allfree[rand.Intn(len(allfree))], nocaseFilter(content))
	} else {
		payload.Fill(allfree[rand.Intn(len(allfree))], content)
	}
	return nil
}
