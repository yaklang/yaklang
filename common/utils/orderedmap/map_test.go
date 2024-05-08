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
