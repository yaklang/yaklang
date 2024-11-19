package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Closu_Freevalue_syntaxflow(t *testing.T) {
	code := `package example

import (
	"flag"
	"log"

	"database/sql"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)


func main() {
	db, err := sql.Open("mysql","root:root@tcp(127.0.0.1:3306)/test")

	router := gin.Default()
	router.GET("/inject", func(ctx *gin.Context) {
		rows, err := db.Query("11111111111") // db为freevalue，syntaxflow中应该能识别并查找到
	})
	router.Run(Addr)
}
`

	t.Run("freevalue bind syntaxflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			sql?{<fullTypeName>?{have: 'database/sql'}} as $entry;
			$entry.Open <getCall> as $db;
			$db <getMembers> as $output;
			$output.Query as $query;
	`, map[string][]string{
			"output": {"Undefined-db(valid)", "Undefined-err(valid)"},
			"query":  {""},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}
