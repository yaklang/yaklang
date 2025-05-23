# jsonquery

[![Build Status](https://github.com/antchfx/jsonquery/actions/workflows/testing.yml/badge.svg)](https://github.com/antchfx/jsonquery/actions/workflows/testing.yml)
[![GoDoc](https://godoc.org/github.com/antchfx/jsonquery?status.svg)](https://godoc.org/github.com/antchfx/jsonquery)
[![Go Report Card](https://goreportcard.com/badge/github.com/antchfx/jsonquery)](https://goreportcard.com/report/github.com/antchfx/jsonquery)

# Overview

[jsonquery](https://github.com/antchfx/jsonquery) is XPath query package for JSON document depended on [xpath](https://github.com/antchfx/xpath) package, writing in go.

jsonquery helps you easy to extract any data from JSON using XPath query without using pre-defined object structure to unmarshal in go, saving your time.

- [htmlquery](https://github.com/antchfx/htmlquery) - XPath query package for HTML document

- [xmlquery](https://github.com/antchfx/xmlquery) - XPath query package for XML document.

### Install Package

```
go get github.com/antchfx/jsonquery
```

## Get Started

The below code may be help your understand what it does. We don't need pre-defined structure or using regexp to extract some data in JSON file, gets any data is easy and fast in jsonquery now.

Using an xpath like syntax to access specific fields of a json structure.

```go
// https://go.dev/play/p/vqoD_jWryKY
package main

import (
	"fmt"
	"strings"

	"github.com/antchfx/jsonquery"
)

func main() {
	s := `{
            "person":{
               "name":"John",
               "age":31,
               "female":false,
               "city":null,
               "hobbies":[
                  "coding",
                  "eating",
                  "football"
               ]
            }
         }`
	doc, err := jsonquery.Parse(strings.NewReader(s))
	if err != nil {
		panic(err)
	}
	// xpath query
	age := jsonquery.FindOne(doc, "age")
	// or
	age = jsonquery.FindOne(doc, "person/age")
	fmt.Printf("%#v[%T]\n", age.Value(), age.Value()) // prints 31[float64]

	hobbies := jsonquery.FindOne(doc, "//hobbies")
	fmt.Printf("%#v\n", hobbies.Value()) // prints []interface {}{"coding", "eating", "football"}
	firstHobby := jsonquery.FindOne(doc, "//hobbies/*[1]")
	fmt.Printf("%#v\n", firstHobby.Value()) // "coding"
}
```

Iterating over a json structure.

```go
// https://go.dev/play/p/vwXQKTCLdVK
package main

import (
	"fmt"
	"strings"

	"github.com/antchfx/jsonquery"
)

func main() {
	s := `{
	"name":"John",
	"age":31,
	"female":false,
	"city":null
	}`
	doc, err := jsonquery.Parse(strings.NewReader(s))
	if err != nil {
		panic(err)
	}
	// iterate all json objects from child ndoes.
	for _, n := range doc.ChildNodes() {
		fmt.Printf("%s: %v[%T]\n", n.Data, n.Value(), n.Value())
	}
}
```

Output:

```
name: John[string]
age: 31[float64]
female: false[bool]
city: <nil>[<nil>]
```

The default Json types and Go types are:

| JSON    | jsonquery(go) |
| ------- | ------------- |
| object  | interface{}   |
| string  | string        |
| number  | float64       |
| boolean | bool          |
| array   | []interface{} |
| null    | nil           |

For more information about JSON & Go see the https://go.dev/blog/json

## Getting Started

#### Load JSON from URL.

```go
doc, err := jsonquery.LoadURL("http://www.example.com/feed?json")
```

#### Load JSON from string.

```go
s :=`{
    "name":"John",
    "age":31,
    "city":"New York"
    }`
doc, err := jsonquery.Parse(strings.NewReader(s))
```

#### Load JSON from io.Reader.

```go
f, err := os.Open("./books.json")
doc, err := jsonquery.Parse(f)
```

#### Parse JSON array

```go
s := `[1,2,3,4,5,6]`
doc, _ := jsonquery.Parse(strings.NewReader(s))
list := jsonquery.Find(doc, "*")
for _, n := range list {
	fmt.Print(n.Value().(float64))
}
```

// Output: `1,2,3,4,5,6`

#### Convert JSON object to XML file

```go
s := `[{"name":"John", "age":31, "female":false, "city":null}]`
doc, _ := jsonquery.Parse(strings.NewReader(s))
fmt.Println(doc.OutputXML())
```

### Methods

#### FindOne()

```go
n := jsonquery.FindOne(doc,"//a")
```

#### Find()

```go
list := jsonquery.Find(doc,"//a")
```

#### QuerySelector()

```go
n := jsonquery.QuerySelector(doc, xpath.MustCompile("//a"))
```

#### QuerySelectorAll()

```go
list :=jsonquery.QuerySelectorAll(doc, xpath.MustCompile("//a"))
```

#### Query()

```go
n, err := jsonquery.Query(doc, "*")
```

#### QueryAll()

```go
list, err := jsonquery.QueryAll(doc, "*")
```

#### Query() vs FindOne()

- `Query()` will return an error if give xpath query expr is not valid.

- `FindOne` will panic error and interrupt your program if give xpath query expr is not valid.

#### OutputXML()

Convert current JSON object to XML format.

## Example of how to convert JSON object to XML file

```json
{
  "store": {
    "book": [
      {
        "id": 1,
        "category": "reference",
        "author": "Nigel Rees",
        "title": "Sayings of the Century",
        "price": 8.95
      },
      {
        "id": 2,
        "category": "fiction",
        "author": "Evelyn Waugh",
        "title": "Sword of Honour",
        "price": 12.99
      },
      {
        "id": 3,
        "category": "fiction",
        "author": "Herman Melville",
        "title": "Moby Dick",
        "isbn": "0-553-21311-3",
        "price": 8.99
      },
      {
        "id": 4,
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
```

```go
doc, err := jsonquery.Parse(strings.NewReader(s))
if err != nil {
	panic(err)
}
fmt.Println(doc.OutputXML())
```

Output the below XML:

```xml
<?xml version="1.0" encoding="utf-8"?>
<root>
  <expensive>10</expensive>
  <store>
    <bicycle>
      <color>red</color>
      <price>19.95</price>
    </bicycle>
    <book>
      <author>Nigel Rees</author>
      <category>reference</category>
      <id>1</id>
      <price>8.95</price>
      <title>Sayings of the Century</title>
    </book>
    <book>
      <author>Evelyn Waugh</author>
      <category>fiction</category>
      <id>2</id>
      <price>12.99</price>
      <title>Sword of Honour</title>
    </book>
    <book>
      <author>Herman Melville</author>
      <category>fiction</category>
      <id>3</id>
      <isbn>0-553-21311-3</isbn>
      <price>8.99</price>
      <title>Moby Dick</title>
    </book>
    <book>
      <author>J. R. R. Tolkien</author>
      <category>fiction</category>
      <id>4</id>
      <isbn>0-395-19395-8</isbn>
      <price>22.99</price>
      <title>The Lord of the Rings</title>
    </book>
  </store>
</root>
```

## XPath Tests

| Query                               | Matched | Native Value Types       | Native Values                                                                                                               |
| ----------------------------------- | ------- | ------------------------ | --------------------------------------------------------------------------------------------------------------------------- |
| `//book`                            | 1       | []interface{}            | `{"book": [{"id":1,... }, {"id":2,... }, {"id":3,... }, {"id":4,... }]}`                                                    |
| `//book/*`                          | 4       | [map[string]interface{}] | `{"id":1,... }`, `{"id":2,... }`, `{"id":3,... }`, `{"id":4,... }`                                                          |
| `//*[price<12.99]`                  | 2       | [map[string]interface{}] | `{"id":1,...}`, `{"id":3,...}`                                                                                              |
| `//book/*/author`                   | 4       | []string                 | `{"author": "Nigel Rees"}`, `{"author": "Evelyn Waugh"}`, `{"author": "Herman Melville"}`, `{"author": "J. R. R. Tolkien"}` |
| `//book/*[last()]`                  | 1       | map[string]interface {}  | `{"id":4,...}`                                                                                                              |
| `//book/*[2]`                       | 1       | map[string]interface{}   | `{"id":2,...}`                                                                                                              |
| `//*[isbn]`                         | 2       | [map[string]interface{}] | `{"id":3,"isbn":"0-553-21311-3",...}`,`{"id":4,"isbn":"0-395-19395-8",...}`                                                 |
| `//*[isbn='0-553-21311-3']`         | 1       | map[string]interface{}   | `{"id":3,"isbn":"0-553-21311-3",...}`                                                                                       |
| `//bicycle`                         | 1       | map[string]interface {}  | `{"bicycle":{"color":...,}}`                                                                                                |
| `//bicycle/color[text()='red']`     | 1       | map[string]interface {}  | `{"color":"red"}`                                                                                                           |
| `//*/category[contains(.,'refer')]` | 1       | string                   | `{"category": "reference"}`                                                                                                 |
| `//price[.=22.99]`                  | 1       | float64                  | `{"price": 22.99}`                                                                                                          |
| `//expensive/text()`                | 1       | string                   | `10`                                                                                                                        |

For more supports XPath feature and function see https://github.com/antchfx/xpath
