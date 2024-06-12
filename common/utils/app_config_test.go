package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

type TestStruct1 struct {
	Name string `app:"name:name,id:1"`
	Age  int    `app:"name:age,id:2"`
	Sex  string `app:"name:sex,id:3"`
}

func TestImportAppConfig(t *testing.T) {
	cfg, err := ParseAppTagToOptions(&TestStruct1{}, map[string]string{
		"name": "default:张三",
		"age":  "default:99",
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(cfg), 3)
	assert.Equal(t, cfg[0].Name, "name")
	assert.Equal(t, cfg[0].DefaultValue, "张三")

	ins := TestStruct1{}
	data := map[string]string{
		"age": "100",
		"sex": "女",
	}
	err = ImportAppConfigToStruct(&ins, data)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Age != 100 || ins.Sex != "女" {
		t.Errorf("ApplyAppConfig failed")
	}
}

func TestExportAppConfigToMap(t *testing.T) {
	data := &TestStruct1{
		Name: "张三",
		Age:  1,
		Sex:  "男",
	}
	res, err := ExportAppConfigToMap(data)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "张三", res["name"])
	assert.Equal(t, "1", res["age"])
	assert.Equal(t, "男", res["sex"])
}
