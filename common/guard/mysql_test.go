package guard

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestGetMySQLServerDetails(t *testing.T) {
	details := GetMySQLServerDetails(context.Background())
	spew.Dump(details)
}
