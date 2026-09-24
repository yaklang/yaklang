//go:build ignore

package main

import (
	"database/sql"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	"os"
)

func main() {
	db, err := sql.Open("mysql", "m2:fixture-only@tcp(127.0.0.1:13306)/m2")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	s, err := db.Prepare("SELECT CAST(? AS SIGNED), ?, ?")
	if err != nil {
		panic(err)
	}
	defer s.Close()
	var rows []any
	for _, n := range []int64{17, -9} {
		var v int64
		var text string
		var empty any
		err = s.QueryRow(n, "go fixture", nil).Scan(&v, &text, &empty)
		if err != nil {
			panic(err)
		}
		rows = append(rows, []any{v, text, empty})
	}
	if err = json.NewEncoder(os.Stdout).Encode(rows); err != nil {
		panic(err)
	}
}
