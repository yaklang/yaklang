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

func TestAppConfig(t *testing.T) {
	cfg, err := LoadAppConfig(&TestStruct1{}, map[string]string{
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
	err = ApplyAppConfig(&ins, data)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Age != 100 || ins.Sex != "女" {
		t.Errorf("ApplyAppConfig failed")
	}
}
