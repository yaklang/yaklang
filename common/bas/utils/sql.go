// Package utils
// @Author bcy2007  2023/9/18 16:29
package utils

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"

	"github.com/yaklang/yaklang/common/utils"
)

type DBEngine interface {
	Connect() error
	Query(string) error
	Exec(string) error
}

type MySQLEngine struct {
	Username string
	Password string
	Address  string
	Db       string

	Conn *sql.DB
}

func (mySQLEngine *MySQLEngine) Connect() error {
	dataSourceName := fmt.Sprintf("%v:%v@tcp(%v)/%v", mySQLEngine.Username, mySQLEngine.Password, mySQLEngine.Address, mySQLEngine.Db)
	conn, err := sql.Open("mysql", dataSourceName)
	if err != nil {
		return utils.Errorf("mysql connect error: %v", err)
	}
	mySQLEngine.Conn = conn
	return nil
}

func (mySQLEngine *MySQLEngine) Query(queryStr string) (*sql.Rows, error) {
	if mySQLEngine.Conn == nil {
		return nil, utils.Error("connection not build")
	}
	result, err := mySQLEngine.Conn.Query(queryStr)
	if err != nil {
		return nil, utils.Errorf("mysql query %v error: %v", queryStr, err)
	}
	return result, nil
}
