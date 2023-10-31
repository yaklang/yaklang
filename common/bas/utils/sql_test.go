// Package utils
// @Author bcy2007  2023/9/18 16:41
package utils

import (
	"database/sql"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	"testing"
)

type RuleFormat struct {
	RuleID  int    `json:"ruleId"`
	Content string `json:"content"`
}

func TestDBOpen(t *testing.T) {
	db, err := sql.Open("mysql", "xiaozhi:xiaozhi.4dogs.cn@tcp(192.168.0.61:33306)/smart")
	//t.Log(db, err)
	if err != nil {
		t.Error(err)
		return
	}
	defer db.Close()
	result, err := db.Query("select id,content from bas_rules limit 10;")
	if err != nil {
		t.Error(err)
		return
	}
	defer result.Close()
	columns, err := result.Columns()
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(columns)
	rules := make([]RuleFormat, 0)
	for result.Next() {
		var id int
		var content string
		if err := result.Scan(&id, &content); err != nil {
			t.Error(err)
			return
		}
		//t.Log(id, content)
		rules = append(rules, RuleFormat{RuleID: id, Content: content})
	}
	ruleJson, _ := json.Marshal(rules)
	t.Log(string(ruleJson))
	err = WriteFile("/Users/chenyangbao/rules.json", ruleJson)
	if err != nil {
		t.Error(err)
	}
}
