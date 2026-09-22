// Derived from aquasecurity/go-dep-parser, MIT. See LICENSE.
package binary_test

import (
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/sca/internal/testcheck"
	"testing"

	assert "github.com/yaklang/yaklang/common/sca/internal/testcheck"
	require "github.com/yaklang/yaklang/common/sca/internal/testcheck"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/golang/binary"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

type ctxFile struct {
	*testcheck.FixtureFile
	ctx context.Context
}

func (c ctxFile) Context() context.Context { return c.ctx }

// Sums are from debug/buildinfo.Read on the frozen fixtures, not from a
// ScanReport dump. Buildinfo has no licenses or line numbers.
var elfWant = []types.Library{
	{
		Name: "github.com/aquasecurity/go-pep440-version", Version: "v0.0.0-20210121094942-22b2f8951d46",
		Verification: "h1:vmXNl+HDfqqXgr0uY1UgK1GAhps8nbAAtqHNBcgyf+4=", DeclaredName: "github.com/aquasecurity/go-pep440-version",
		DeclaredVersion: "v0.0.0-20210121094942-22b2f8951d46", Evidence: "binary", ID: "github.com/aquasecurity/go-pep440-version",
	},
	{
		Name: "github.com/aquasecurity/go-version", Version: "v0.0.0-20210121072130-637058cfe492",
		Verification: "h1:rcEG5HI490FF0a7zuvxOxen52ddygCfNVjP0XOCMl+M=", DeclaredName: "github.com/aquasecurity/go-version",
		DeclaredVersion: "v0.0.0-20210121072130-637058cfe492", Evidence: "binary", ID: "github.com/aquasecurity/go-version",
	},
	{
		Name: "golang.org/x/xerrors", Version: "v0.0.0-20200804184101-5ec99f83aff1",
		Verification: "h1:go1bK/D/BFZV2I8cIQd1NKEZ+0owSTG1fDTci4IqFcE=", DeclaredName: "golang.org/x/xerrors",
		DeclaredVersion: "v0.0.0-20200804184101-5ec99f83aff1", Evidence: "binary", ID: "golang.org/x/xerrors",
	},
}

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		inputFile string
		want      []types.Library
		wantErr   string
	}{
		{
			name:      "ELF",
			inputFile: "testdata/test.elf",
			want:      elfWant,
		},
		{
			name:      "PE",
			inputFile: "testdata/test.exe",
			want:      elfWant,
		},
		{
			name:      "Mach-O",
			inputFile: "testdata/test.macho",
			want:      elfWant,
		},
		{
			name:      "with replace directive",
			inputFile: "testdata/replace.elf",
			want: []types.Library{
				{
					Name:            "github.com/davecgh/go-spew",
					Version:         "v1.1.1",
					DeclaredName:    "github.com/davecgh/go-spew",
					DeclaredVersion: "v1.1.1",
					Evidence:        "binary",
					ID:              "github.com/davecgh/go-spew",
				},
				{
					Name:            "github.com/go-sql-driver/mysql",
					Version:         "v1.5.0",
					DeclaredName:    "github.com/go-sql-driver/mysql",
					DeclaredVersion: "v0.0.0-00010101000000-000000000000",
					Source:          "github.com/go-sql-driver/mysql",
					Evidence:        "binary",
					ID:              "github.com/go-sql-driver/mysql",
				},
			},
		},
		{
			name:      "sad path",
			inputFile: "testdata/dummy",
			wantErr:   "unrecognized executable format",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := fixtures.OpenReader(tt.inputFile)
			require.NoError(t, err)
			defer f.Close()

			got, _, err := binary.NewParser().Parse(nil, f)
			if tt.wantErr != "" {
				require.NotNil(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
			for _, lib := range got {
				if lib.License != "" || len(lib.Locations) != 0 {
					t.Fatalf("invented license or line: %+v", lib)
				}
			}
		})
	}
}

func TestParseBudget(t *testing.T) {
	f, err := fixtures.OpenReader("testdata/test.elf")
	require.NoError(t, err)
	defer f.Close()
	l, err := (budget.Limits{MaxResultBytes: 48}).Normalize()
	require.NoError(t, err)
	ctx := budget.Bind(context.Background(), l)
	_, _, err = binary.NewParser().Parse(nil, ctxFile{FixtureFile: f, ctx: ctx})
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("small budget: %v", err)
	}
}
