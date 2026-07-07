package guard

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestGetMySQLServerDetails(t *testing.T) {
	details := GetMySQLServerDetails(context.Background())
	spew.Dump(details)
}
