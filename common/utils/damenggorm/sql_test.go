package damenggorm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdentifierNormalizationDoesNotChangePayloads(t *testing.T) {
	for _, test := range []struct{ query, want string }{
		{`SELECT "string" FROM "ir_types" WHERE "program_name"=?`, `SELECT "STRING" FROM "IR_TYPES" WHERE "PROGRAM_NAME"=?`},
		{`SELECT '"source"', 'it''s "quoted"' FROM "ir_codes"`, `SELECT '"source"', 'it''s "quoted"' FROM "IR_CODES"`},
		{"SELECT \"id\" -- \"keep\"\nFROM \"ir_codes\" /* \"keep\" */", "SELECT \"ID\" -- \"keep\"\nFROM \"IR_CODES\" /* \"keep\" */"},
	} {
		require.Equal(t, test.want, NormalizeQuery(test.query))
	}
}

func TestMissingSchemaIsRejectedBeforeConnecting(t *testing.T) {
	for _, raw := range []string{"dm://user:password@127.0.0.1:1", "dm://user:password@127.0.0.1:1?schema=lowercase", "sqlite:///tmp/unowned.db"} {
		db, err := OpenDBContext(context.Background(), raw)
		require.Nil(t, db)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "password")
	}
}
