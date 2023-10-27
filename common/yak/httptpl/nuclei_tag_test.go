package httptpl

import (
	"errors"
	"testing"
)

func TestNucleiTag(t *testing.T) {
	res, err := ExecNucleiTag(`http://{{HostName}}:80/aaa`, map[string]any{
		"HostName": "baidu.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != "http://baidu.com:80/aaa" {
		t.Fatal(errors.New("ExecNucleiTag error"))
	}
	// 集束炸弹
	fuzzRes, err := FuzzNucleiTag("{{account}}:{{username}}-{{password}}", map[string]any{
		"account": "account",
	}, map[string][]string{
		"username": {"admin", "root"},
		"password": {"123456", "000000"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	expect := []string{"account:admin-123456", "account:root-123456", "account:admin-000000", "account:root-000000"}
	for i, r := range fuzzRes {
		if string(r) != expect[i] {
			t.Fatal("FuzzNucleiTag error")
		}
	}
	// 草叉模式
	fuzzRes, err = FuzzNucleiTag("{{account}}:{{username}}-{{password}}", map[string]any{
		"account": "account",
	}, map[string][]string{
		"username": {"admin", "root"},
		"password": {"123456", "000000"},
	}, "pitchfork")
	if err != nil {
		t.Fatal(err)
	}
	expect = []string{"account:admin-123456", "account:root-000000"}
	for i, r := range fuzzRes {
		if string(r) != expect[i] {
			t.Fatal("FuzzNucleiTag error")
		}
	}
}
