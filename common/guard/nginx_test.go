package guard

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestGetNginxDetail(t *testing.T) {
	details := GetNginxDetail(context.Background())
	spew.Dump(details)
}
