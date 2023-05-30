package httptpl

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"testing"
)

func TestYakExtractor_Execute(t *testing.T) {
	for index, extractor := range [][]any{
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<!DOCTYPE html>
<html></html>`,
			&YakExtractor{
				Name:   "k1",
				Type:   "regex",
				Groups: []string{`DOCTYPE \w{4}`},
			},
			"k1",
			"DOCTYPE html",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<!DOCTYPE html>
<html></html>`,
			&YakExtractor{
				Name:             "k1",
				Type:             "regex",
				RegexpMatchGroup: []int{1},
				Groups:           []string{`DOCTYPE (\w{4})`},
			},
			"k1",
			"html",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<!DOCTYPE html>
<html></html>`,
			&YakExtractor{
				Name:             "k1",
				Type:             "regex",
				RegexpMatchGroup: []int{1},
				Scope:            "header",
				Groups:           []string{`DOCTYPE (\w{4})`},
			},
			"k1",
			"",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

{"abc": "12312312", "ccc": 123}`,
			&YakExtractor{
				Name:   "k1",
				Type:   "json",
				Scope:  "body",
				Groups: []string{`.abc`},
			},
			"k1",
			"12312312",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

{"abc": "12312312", "ccc": 123}`,
			&YakExtractor{
				Name:   "k1",
				Type:   "json",
				Scope:  "body",
				Groups: []string{`.ccc`},
			},
			"k1",
			"123",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<html>
	<head><title>ABC</title></head>
</html>`,
			&YakExtractor{
				Name:   "k1",
				Type:   "xpath",
				Scope:  "body",
				Groups: []string{`//title/text()`},
			},
			"k1",
			"ABC",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<?xml version="1.0" encoding="UTF-8"?>
<products>
  <product>
    <name>iPhone 13</name>
    <price>999.00</price>
    <description>The latest iPhone from Apple.</description>
    <reviews>
      <review>
        <rating>4.5</rating>
        <comment>Great phone, but a bit expensive.</comment>
      </review>
      <review>
        <rating>3.0</rating>
        <comment>Not impressed, I expected more.</comment>
      </review>
    </reviews>
  </product>
  <product>
    <name>Samsung Galaxy S21</name>
    <price>799.00</price>
    <description>The latest Galaxy phone from Samsung.</description>
    <reviews>
      <review>
        <rating>5.0</rating>
        <comment>Amazing phone, great value for money.</comment>
      </review>
      <review>
        <rating>4.0</rating>
        <comment>Good phone, but battery life could be better.</comment>
      </review>
    </reviews>
  </product>
</products>
`,
			&YakExtractor{
				Name:   "k1",
				Type:   "xpath",
				Scope:  "body",
				Groups: []string{`/products/product[name='Samsung Galaxy S21']/price/text()`},
			},
			"k1",
			"799.00",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<products>
  <product>
    <name>iPhone 13</name>
    <price>999.00</price>
    <description>The latest iPhone from Apple.</description>
    <reviews>
      <review>
        <rating>4.5</rating>
        <comment>Great phone, but a bit expensive.</comment>
      </review>
      <review>
        <rating>3.0</rating>
        <comment>Not impressed, I expected more.</comment>
      </review>
    </reviews>
  </product>
  <product>
    <name>Samsung Galaxy S21</name>
    <price>799.00</price>
    <description>The latest Galaxy phone from Samsung.</description>
    <reviews>
      <review>
        <rating>5.0</rating>
        <comment>Amazing phone, great value for money.</comment>
      </review>
      <review>
        <rating>4.0</rating>
        <comment>Good phone, but battery life could be better.</comment>
      </review>
    </reviews>
  </product>
</products>
`,
			&YakExtractor{
				Name:   "cc",
				Type:   "xpath",
				Scope:  "body",
				Groups: []string{`/products/product[name='Samsung Galaxy S21']/price/text()`},
			},
			"cc",
			"799.00",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<products>
  <product>
    <name>iPhone 13</name>
    <price>999.00</price>
    <description>The latest iPhone from Apple.</description>
    <reviews>
      <review>
        <rating>4.5</rating>
        <comment>Great phone, but a bit expensive.</comment>
      </review>
      <review>
        <rating>3.0</rating>
        <comment>Not impressed, I expected more.</comment>
      </review>
    </reviews>
  </product>
  <product>
    <name>Samsung Galaxy S21</name>
    <price>799.00</price>
    <description>The latest Galaxy phone from Samsung.</description>
    <reviews>
      <review>
        <rating>5.0</rating>
        <comment>Amazing phone, great value for money.</comment>
      </review>
      <review>
        <rating>4.0</rating>
        <comment>Good phone, but battery life could be better.</comment>
      </review>
    </reviews>
  </product>
</products>
`,
			&YakExtractor{
				Name:   "cc",
				Type:   "nuclei-dsl",
				Scope:  "body",
				Groups: []string{`dump(body); contains(body, "rating>4.0") ? "abc": "def"`},
			},
			"cc",
			"abc",
		},
	} {
		data, extractor, key, value := extractor[0].(string), extractor[1].(*YakExtractor), extractor[2].(string), extractor[3].(string)
		vars, err := extractor.Execute([]byte(data))
		if err != nil {
			log.Infof("INDEX: %v failed: %v", index, err)
			panic(err)
		}
		ret, _ := vars[key]
		if toString(ret) != value {
			log.Infof("INDEX: %v failed: %v", index, spew.Sdump(vars))
			panic("failed")
		}
		spew.Dump(vars)
	}
}

func TestExtractKValFromResponse(t *testing.T) {
	for index, extractor := range [][]any{
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
`,
			"charset",
			"utf-8",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=11112
`,
			"JSE",
			"1111",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=%251; CCC=11112
`,
			"JSE",
			"%1",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=A12
`,
			"CCC",
			"A12",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=A12

{
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
    "expensive": 10,
	"cc1": 111
}
`,
			"cc1",
			"111",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=A12

{
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
    "expensive": 10,
	"cc1": 111
}
`,
			"expensive",
			"10",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=A12

asdfjkasdjklfasjdf
expensive=10
as
12
312
31
23


`,
			"expensive",
			"10",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8
Cookie: JSE=1111; CCC=A12

asdfjkasdjklfasjdf
expensive=10
"abcc": 10
as
12
312
31
23


`,
			"abcc",
			"10",
		},
	} {
		_ = index
		results := ExtractKValFromResponse([]byte(extractor[0].(string)))
		key, value := extractor[1].(string), extractor[2].(string)
		if toString(results[key]) != toString(value) {
			log.Infof("INDEX: %v failed: %v", index, spew.Sdump(results))
			t.FailNow()
		}
	}
}

// lack testcase for kval and xpath attribute
func TestYakExtractor_REGEXP(t *testing.T) {
	for index, extractor := range [][]any{
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<!DOCTYPE html>
<html></html>`,
			&YakExtractor{
				Name:   "k1",
				Type:   "regex",
				Groups: []string{`DOCTYPE \w{4}`},
			},
			"k1",
			"DOCTYPE html",
		},
	} {
		data, extractor, key, value := extractor[0].(string), extractor[1].(*YakExtractor), extractor[2].(string), extractor[3].(string)
		vars, err := extractor.Execute([]byte(data))
		if err != nil {
			log.Infof("INDEX: %v failed: %v", index, err)
			panic(err)
		}
		ret, _ := vars[key]
		if toString(ret) != value {
			log.Infof("INDEX: %v failed: %v", index, spew.Sdump(vars))
			panic("failed")
		}
		spew.Dump(vars)
	}
}

func TestYakExtractor_XPATH_ATTR(t *testing.T) {
	for index, extractor := range [][]any{
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<html>
	<head><title value="999">ABC</title></head>
</html>`,
			&YakExtractor{
				Name:           "k1",
				Type:           "xpath",
				Scope:          "body",
				XPathAttribute: "value",
				Groups:         []string{`//title`},
			},
			"k1",
			"999",
		},
		{
			`HTTP/1.1 200 Ok
Content-Type: text/html; charset=utf-8

<html>
	<head><title value="999">ABC</title></head>
</html>`,
			&YakExtractor{
				Type:           "xpath",
				Scope:          "body",
				XPathAttribute: "value",
				Groups:         []string{`//title`},
			},
			"data0",
			"999",
		},
	} {
		data, extractor, key, value := extractor[0].(string), extractor[1].(*YakExtractor), extractor[2].(string), extractor[3].(string)
		vars, err := extractor.Execute([]byte(data))
		if err != nil {
			log.Infof("INDEX: %v failed: %v", index, err)
			panic(err)
		}
		ret, _ := vars[key]
		if toString(ret) != value {
			log.Infof("INDEX: %v failed: %v", index, spew.Sdump(vars))
			panic("failed")
		}
		spew.Dump(vars)
	}
}

func TestYakExtractor_KVAL(t *testing.T) {
	for index, extractor := range [][]any{
		{
			`HTTP/1.1 200 OK
Date: Mon, 23 May 2005 22:38:34 GMT
Content-Type: text/html; charset=UTF-8
Content-Encoding: UTF-8

<html><!doctype html>
<html>
<body>
  <div id="result">%d</div>
</body>
</html></html>`,
			&YakExtractor{
				Name:   "k1",
				Type:   "kv",
				Groups: []string{`id`},
			},
			"k1",
			"result",
		},
	} {
		data, extractor, key, value := extractor[0].(string), extractor[1].(*YakExtractor), extractor[2].(string), extractor[3].(string)
		vars, err := extractor.Execute([]byte(data))
		if err != nil {
			log.Infof("INDEX: %v failed: %v", index, err)
			panic(err)
		}
		ret, _ := vars[key]
		if toString(ret) != value {
			log.Infof("INDEX: %v failed: %v", index, spew.Sdump(vars))
			panic("failed")
		}
		spew.Dump(vars)
	}
}
