package jsonquery

import (
	"strings"
	"testing"

	"github.com/antchfx/xpath"
	"github.com/stretchr/testify/require"
)

func BenchmarkSelectorCache(b *testing.B) {
	DisableSelectorCache = false
	for i := 0; i < b.N; i++ {
		getQuery("/AAA/BBB/DDD/CCC/EEE/ancestor::*")
	}
}

func BenchmarkDisableSelectorCache(b *testing.B) {
	DisableSelectorCache = true
	for i := 0; i < b.N; i++ {
		getQuery("/AAA/BBB/DDD/CCC/EEE/ancestor::*")
	}
}

func TestNavigator(t *testing.T) {
	s := `{
		"name":"John",
		"age":30,
		"cars": [
			{ "name":"Ford", "models":[ "Fiesta", "Focus", "Mustang" ] },
			{ "name":"BMW", "models":[ "320", "X3", "X5" ] },
			{ "name":"Fiat", "models":[ "500", "Panda" ] }
		]
	 }`
	doc, err := parseString(s)
	require.NoError(t, err)

	nav := CreateXPathNavigator(doc)
	nav.MoveToRoot()
	require.Equal(t, xpath.RootNode, nav.NodeType(), "node type is not RootNode")

	// Move to first child(age).
	require.True(t, nav.MoveToChild())
	require.Equal(t, "age", nav.Current().Data)
	require.Equal(t, float64(30), nav.GetValue())

	// Move to next sibling node(cars).
	require.True(t, nav.MoveToNext())
	require.Equal(t, "cars", nav.Current().Data)

	m := make(map[string][]string)
	// Move to cars child node.
	cur := nav.Copy()
	for ok := nav.MoveToChild(); ok; ok = nav.MoveToNext() {
		// Move to <element> node.
		// <element><models>...</models><name>Ford</name></element>
		cur1 := nav.Copy()
		var name string
		var models []string
		// name || models
		for ok := nav.MoveToChild(); ok; ok = nav.MoveToNext() {
			cur2 := nav.Copy()
			n := nav.Current()
			require.NotNil(t, n)
			if n.Data == "name" {
				name = n.InnerText()
			} else {
				for ok := nav.MoveToChild(); ok; ok = nav.MoveToNext() {
					cur3 := nav.Copy()
					models = append(models, nav.Value())
					nav.MoveTo(cur3)
				}
			}
			nav.MoveTo(cur2)
		}
		nav.MoveTo(cur1)
		m[name] = models
	}

	nav.MoveTo(cur)
	// move to name.
	require.True(t, nav.MoveToNext())
	// move to cars
	require.True(t, nav.MoveToPrevious())
	require.Equal(t, "cars", nav.Current().Data)
	// move to age.
	require.True(t, nav.MoveToFirst())
	require.Equal(t, "age", nav.Current().Data)

	nav.MoveToParent()
	require.Equal(t, DocumentNode, nav.Current().Type)
}

func TestToXML(t *testing.T) {
	s := `{
	"name":"John",
	"age":31,
	"female":false
  }`
	doc, err := Parse(strings.NewReader(s))
	require.NoError(t, err)

	expected := `<?xml version="1.0" encoding="utf-8"?><root><age>31</age><female>false</female><name>John</name></root>`
	require.Equal(t, expected, doc.OutputXML())
}

func TestArrayToXML(t *testing.T) {
	s := `[1,2,3,4]`
	doc, err := Parse(strings.NewReader(s))
	require.NoError(t, err)

	expected := `<?xml version="1.0" encoding="utf-8"?><root><1>1</1><2>2</2><3>3</3><4>4</4></root>`
	require.Equal(t, expected, doc.OutputXML())
}

func TestNestToArray(t *testing.T) {
	s := `{
		"address": {
		  "city": "Nara",
		  "postalCode": "630-0192",
		  "streetAddress": "naist street"
		},
		"age": 26,
		"name": "John",
		"phoneNumbers": [
		  {
			"number": "0123-4567-8888",
			"type": "iPhone"
		  },
		  {
			"number": "0123-4567-8910",
			"type": "home"
		  }
		]
	  }`
	doc, err := Parse(strings.NewReader(s))
	require.NoError(t, err)

	expected := `<?xml version="1.0" encoding="utf-8"?><root><address><city>Nara</city><postalCode>630-0192</postalCode><streetAddress>naist street</streetAddress></address><age>26</age><name>John</name><phoneNumbers><number>0123-4567-8888</number><type>iPhone</type></phoneNumbers><phoneNumbers><number>0123-4567-8910</number><type>home</type></phoneNumbers></root>`
	require.Equal(t, expected, doc.OutputXML())
}

func TestQuery(t *testing.T) {
	doc, err := Parse(strings.NewReader(BooksExample))
	require.NoError(t, err)

	q := "/store/bicycle"
	n := FindOne(doc, q)
	require.NotNil(t, n)

	q = "/store/bicycle/color"
	n = FindOne(doc, q)
	require.NotNil(t, n)
	require.Equal(t, "color", n.Data)
}

func TestQueryWhere(t *testing.T) {
	doc, err := Parse(strings.NewReader(BooksExample))
	require.NoError(t, err)

	// for number
	q := "//*[price<=12.99]"
	list := Find(doc, q)
	require.Len(t, list, 3)

	// for string
	q = "//*/isbn[text()='0-553-21311-3']"
	n := FindOne(doc, q)
	require.NotNil(t, n)
	require.Equal(t, "isbn", n.Data)
}

func TestStringRepresentation(t *testing.T) {
	s := `{
		"a": "a string",
		"b": 3.1415,
		"c": true,
		"d": {
		  "d1": 1,
		  "d2": "foo",
		  "d3": true,
		  "d4": null
		},
		"e": ["master", 42, true],
		"f": 1690193829
	}`
	doc, err := Parse(strings.NewReader(s))
	require.NoError(t, err)

	expected := map[string]string{
		"a": "a string",
		"b": "3.1415",
		"c": "true",
		"d": `{"d1":1,"d2":"foo","d3":true,"d4":null}`,
		"e": `["master",42,true]`,
		"f": "1690193829",
	}

	nn := CreateXPathNavigator(doc)
	hasData := nn.MoveToChild()
	require.True(t, hasData)
	for hasData {
		require.NotNil(t, nn.Current())
		name := nn.Current().Data
		require.Equalf(t, expected[name], nn.Value(), "mismatch for node %q", name)
		hasData = nn.MoveToNext()
	}
}

var BooksExample string = `{
	"store": {
	  "book": [
		{
		  "category": "reference",
		  "author": "Nigel Rees",
		  "title": "Sayings of the Century",
		  "price": 8.95
		},
		{
		  "category": "fiction",
		  "author": "Evelyn Waugh",
		  "title": "Sword of Honour",
		  "price": 12.99
		},
		{
		  "category": "fiction",
		  "author": "Herman Melville",
		  "title": "Moby Dick",
		  "isbn": "0-553-21311-3",
		  "price": 8.99
		},
		{
		  "category": "fiction",
		  "author": "J. R. R. Tolkien",
		  "title": "The Lord of the Rings",
		  "isbn": "0-395-19395-8",
		  "price": 22.99
		}
	  ],
	  "bicycle": {
		"color": "red",
		"price": 19.95
	  }
	},
	"expensive": 10
  }
`
