package tlsutils

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSplitBlock(t *testing.T) {
	text := `

	e.Returns = target.Returns
}

type VariableDoc struct {
	Name           string
	TypeStr        string
	Description    str
}

func (e *VariableDoc) Hash() string {
	return codec.Sha512(fmt.Sprint(e.Name, e.TypeStr))
}

type LibDoc struct {
	Name      string
	Functions []*ExportsFunctionDoc
	Variables []*VariableDoc
}

func (l *LibDoc) Hash() string {
	var s []string
	s = append(s, l.Name)

	for _, f := range l.Functions {
		s = append(s, f.Hash())
	}

	for _, f := range l.Variables {
		s = append(s, f.Hash())
	}

	sort.Strings(s)
	return codec.Sha512(strings.Join(s, "|"))
}

`

	test := assert.New(t)
	res, err := SplitBlock([]byte(text), 3)
	if err != nil {
		test.FailNow(err.Error())
	}
	spew.Dump(res)

	raw, err := MergeBlock(res)
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	test.Equal(raw, []byte(text))
}
