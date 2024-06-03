package spacengine

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func TestShodanQuery(t *testing.T) {
	res, err := ShodanQuery("*", "port:8080", 1, 10)
	require.NoError(t, err)
	pass := false
	for result := range res {
		pass = true
		spew.Dump(result)
	}
	if !pass {
		t.Fatal("no result")
	}
}

func TestFofaQuery(t *testing.T) {
	res, err := FofaQuery("user", "pass", "domain=qq.com", 1, 30, 30)
	require.NoError(t, err)
	pass := false
	for result := range res {
		pass = true
		spew.Dump(result)
	}
	if !pass {
		t.Fatal("no result")
	}
}

func TestQuakeQuery(t *testing.T) {
	res, err := QuakeQuery("", "service: http", 1, 30)
	require.NoError(t, err)
	pass := false
	for result := range res {
		pass = true
		spew.Dump(result)
	}
	if !pass {
		t.Fatal("no result")
	}
}

func TestHunterQuery(t *testing.T) {
	res, err := HunterQuery("", `web.title="北京"`, 1, 10, 10)
	pass := false
	require.NoError(t, err)
	for result := range res {
		pass = true
		spew.Dump(result)
	}
	if !pass {
		t.Fatal("no result")
	}
}

func TestZoomEyeQuery(t *testing.T) {
	res, err := ZoomeyeQuery("", "site:baidu.com", 1, 10)
	require.NoError(t, err)
	pass := false
	for result := range res {
		pass = true
		spew.Dump(result)
	}
	if !pass {
		t.Fatal("no result")
	}
}
