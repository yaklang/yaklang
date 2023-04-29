package guard

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestGetNginxDetail(t *testing.T) {
	details := GetNginxDetail(context.Background())
	spew.Dump(details)
}
