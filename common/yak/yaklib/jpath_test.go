package yaklib

import (
	"bytes"
	"github.com/bcicen/jstream"
	"github.com/davecgh/go-spew/spew"
	"reflect"
	"testing"
	"github.com/yaklang/yaklang/common/jsonpath"
)

const data = `{
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

func TestJsonPathBasic(t *testing.T) {
	decoder := jstream.NewDecoder(bytes.NewBuffer([]byte(data)), 0)
	for metaValue := range decoder.Stream() {
		value := metaValue.Value
		spew.Dump(reflect.TypeOf(value))
		rule, err := jsonpath.Read(value, `$..*`)
		if err != nil {
			panic(err)
		}
		spew.Dump(rule)
	}
}
