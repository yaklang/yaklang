package orderedmap

import (
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrderedMapXML(t *testing.T) {
	xmlCode := `<list>
<student id="stu2" name="stu">
	<id>1002</id>
	<name>李四</name>
	<age>21</age>
	<gender>女</gender>
</student>
</list>`
	var m OrderedMap
	err := xml.Unmarshal([]byte(xmlCode), &m)
	require.NoError(t, err)
	t.Logf("%s", m)

	b, err := xml.MarshalIndent(m, "", "  ")
	require.NoError(t, err)
	t.Logf("%s", b)
}

func TestOrderMapCopy(t *testing.T) {
	m := New()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)
	m.Set("d", 4)
	m.Set("e", 5)
	m.Set("f", 6)
	m.Set("g", 7)
	m.Set("h", 8)
	m.Set("i", 9)
	m.Set("j", 10)

	m2 := m.Copy()
	require.Equal(t, m.Keys(), m2.Keys())
}
