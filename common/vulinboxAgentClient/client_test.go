package vulinboxAgentClient

import (
	"context"
	"github.com/yaklang/yaklang/common/vulinbox"
	"math/rand"
	"testing"
	"time"
)

func TestPing(t *testing.T) {
	server, err := vulinbox.NewVulinServer(context.Background(), rand.Intn(55535)+10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(server)
	time.Sleep(time.Second * 3)
	_, err = Connect(server)
	if err != nil {
		t.Fatal(err)
	}

}
