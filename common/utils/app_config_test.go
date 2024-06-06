package utils

import "testing"

type TestStruct1 struct {
	Name string `app:"name:name,default:张三"`
	Age  int    `app:"name:age,default:99"`
	Sex  string `app:"name:sex,default:男"`
}

func TestAppConfig(t *testing.T) {
	cfg, err := LoadAppConfig(&TestStruct1{})
	if err != nil {
		t.Fatal(err)
	}
	ins := TestStruct1{}
	err = ApplyAppConfig(&ins, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Name != "张三" || ins.Age != 99 || ins.Sex != "男" {
		t.Errorf("ApplyAppConfig failed")
	}
}
