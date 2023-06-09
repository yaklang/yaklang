package cui

import (
	utils2 "github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"
)

func TestStatusText_Read(t *testing.T) {
	status := NewStatusText(500*time.Millisecond, []byte("init"))

	data, _ := utils2.ReadWithLen(status, 8)
	if string(data) != "initinit" {
		t.Errorf("expect: %s got %s", "initinit", data)
		t.FailNow()
	}

	status.Update([]byte("updated"))
	data, _ = utils2.ReadWithLen(status, 11)
	if !strings.Contains(string(data), "updated") {
		t.Errorf("expect: %s got %s", "update", data)
		t.FailNow()
	}
}
