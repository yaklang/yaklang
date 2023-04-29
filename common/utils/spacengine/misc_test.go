package spacengine

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestQuery(t *testing.T) {
	var res, err = FofaQuery("huang*com", "8630714*aa1489f", "domain=qq.com", 1, 30)
	if err != nil {
		panic(err)
	}

	for result := range res {
		spew.Dump(result)
	}
}

func TestQuakeQuery(t *testing.T) {
	var res, err = QuakeQuery("245725*5a8c7c65", "service: http", 1, 30)
	if err != nil {
		panic(err)
	}

	for result := range res {
		spew.Dump(result)
	}
}

func TestHunterQuery(t *testing.T) {
	var res, err = HunterQuery("v1ll4n", "1d56544b74dfa1546*9d6056882802e", `web.title="北京"`, 1, 12)
	if err != nil {
		panic(err)
	}
	for result := range res {
		spew.Dump(result)
	}
}

func TestShodamQuery(t *testing.T) {
	var res, err = ShodanQuery("vO5ZsWimJBUwetdI6zqpUnN2aHgdTeEM", "port:8080", 1, 20)
	if err != nil {
		panic(err)
	}
	for result := range res {
		spew.Dump(result)
	}
}