package example

import (
	"flag"
	"log"

	"database/sql"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

var (
	Addr = "0.0.0.0:8080"
)

func init() {
	flag.StringVar(&Addr, "addr", "0.0.0.0:8080", "Server listen address")
	flag.Parse()
}

func main() {
	db, err := sql.Open("mysql",
		"root:root@tcp(127.0.0.1:3306)/test")
	defer db.Close()

	if err != nil {
		panic(err)
	}
	err = db.Ping()
	if err != nil {
		panic(err)
	}
	router := gin.Default()
	router.GET("/inject", func(ctx *gin.Context) {
		var (
			username string
		)
		// source
		id := ctx.Query("id")
		if id == "" {
			id = "1"
		}

		id2 := id + "hhhhhh"
		// sink
		rows, err := db.Query("select username from users where id = " + id2)
		if err != nil {
			log.Panic(err)
		}
		defer rows.Close()

		for rows.Next() {
			err := rows.Scan(&username)
			if err != nil {
				log.Panic(err)
			}
		}

		ctx.String(200, username)
	})
	router.Run(Addr)
}
