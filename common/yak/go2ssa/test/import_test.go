package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestImport(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		import (
			"gorm.io/driver/sqlite"
			"gorm.io/gorm"
			"log"
		)

		func main() {
			db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
			if err != nil {
				log.Fatal("failed to connect to database:", err)
			}
			println(db)
			println(err)
		}

		`, []string{"Undefined-db(valid)", "Undefined-err(valid)"}, t)
	})

	t.Run("struct", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		import (
			"fmt"
			"net/url"
		)

		func main() {
			rawURL := "https://www.example.com:8080/path?query=123#fragment"
			parsedURL, err := url.Parse(rawURL)
			if err != nil {
				fmt.Println("Error parsing URL:", err)
				return
			}

			println(parsedURL.Scheme)
		}

		`, []string{"Undefined-parsedURL.Scheme(valid)"}, t)
	})

	t.Run("struct value", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		import (
			"fmt"
			"net/url"
		)

		func main() {
			u := url.URL{
				Scheme: "https",
				Host:   "www.example.com",
				Path:   "/path",
				RawQuery: "query=123",
			}

			println(u.Scheme)
			println(u.Host)
		}

		`, []string{"\"https\"", "\"www.example.com\""}, t)
	})

	t.Run("struct value-muti", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		import (
			"fmt"
			"net/url"
		)

		func main() {
			u := url.URL{
				Scheme: url.URL{
					Scheme: []string{"https"},
				},
				Host:   "www.example.com",
			}

			println(u.Scheme.Scheme[0])
			println(u.Host)
		}

		`, []string{"\"https\"", "\"www.example.com\""}, t)
	})
}
