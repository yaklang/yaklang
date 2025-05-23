package jsonquery

import (
	"reflect"
	"strings"
	"testing"
)

func parseString(s string) (*Node, error) {
	return Parse(strings.NewReader(s))
}

func TestParseJsonNumberArray(t *testing.T) {
	s := `[1,2,3,4,5,6]`
	doc, err := parseString(s)
	if err != nil {
		t.Fatal(err)
	}

	var values []float64
	for _, n := range doc.ChildNodes() {
		values = append(values, n.Value().(float64))
	}

	expected := []float64{1, 2, 3, 4, 5, 6}

	if p1, p2 := len(values), len(expected); p1 != p2 {
		t.Fatalf("got %d elements but expected %d", p1, p2)
	}
	if !reflect.DeepEqual(values, expected) {
		t.Fatalf("got %v but expected %v", values, expected)
	}
}

func TestParseJsonObject(t *testing.T) {
	s := `{
		"name":"John",
		"age":31,
		"female":false
	}`
	doc, err := parseString(s)
	if err != nil {
		t.Fatal(err)
	}

	m := make(map[string]interface{})

	for _, n := range doc.ChildNodes() {
		m[n.Data] = n.Value()
	}
	expected := []struct {
		name  string
		value interface{}
	}{
		{"name", "John"},
		{"age", float64(31)},
		{"female", false},
	}
	for _, v := range expected {
		if e, g := v.value, m[v.name]; e != g {
			t.Fatalf("expected %s = %v(%T),but %s = %v(%t)", v.name, e, e, v.name, g, g)
		}
	}
}

func TestParseJsonObjectArray(t *testing.T) {
	s := `[
		{"models":[ "Fiesta", "Focus", "Mustang" ] }
	]`
	doc, err := parseString(s)
	if err != nil {
		t.Fatal(err)
	}

	first := doc.FirstChild
	models := first.SelectElement("models")

	if expected := reflect.ValueOf(models.Value()).Kind(); expected != reflect.Slice {
		t.Fatalf("expected models is slice(Array) but got %v", expected)
	}

	expected := []string{"Fiesta", "Focus", "Mustang"}
	var values []string
	for _, v := range models.Value().([]interface{}) {
		values = append(values, v.(string))
	}

	if !reflect.DeepEqual(expected, values) {
		t.Fatalf("expected %v but got %v", expected, values)
	}
}

func TestParseJson(t *testing.T) {
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
	if err != nil {
		t.Fatal(err)
	}
	n := doc.SelectElement("name")
	if n == nil {
		t.Fatal("n is nil")
	}
	if n.NextSibling != nil {
		t.Fatal("next sibling shoud be nil")
	}
	if e, g := "John", n.InnerText(); e != g {
		t.Fatalf("expected %v but %v", e, g)
	}
	cars := doc.SelectElement("cars")
	if e, g := 3, len(cars.ChildNodes()); e != g {
		t.Fatalf("expected %v but %v", e, g)
	}
}

func TestLargeFloat(t *testing.T) {
	s := `{
		"large_number": 365823929453
	 }`
	doc, err := parseString(s)
	if err != nil {
		t.Fatal(err)
	}
	n := doc.SelectElement("large_number")
	if n.Value() != float64(365823929453) {
		t.Fatalf("expected %v but %v", "365823929453", n.InnerText())
	}
}
