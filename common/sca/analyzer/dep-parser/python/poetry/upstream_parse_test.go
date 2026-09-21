// Derived from aquasecurity/go-dep-parser, MIT. See LICENSE.
package poetry

import (
	"fmt"
	"os"
	"testing"

	assert "github.com/yaklang/yaklang/common/sca/internal/testcheck"
	require "github.com/yaklang/yaklang/common/sca/internal/testcheck"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParser_Parse(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		wantLibs []types.Library
		wantDeps []types.Dependency
		wantErr  assert.ErrorAssertionFunc
	}{
		{
			name:     "normal",
			file:     "testdata/poetry_normal.lock",
			wantLibs: poetryNormal,
			wantErr:  assert.NoError,
		},
		{
			name:     "many",
			file:     "testdata/poetry_many.lock",
			wantLibs: poetryMany,
			wantDeps: poetryManyDeps,
			wantErr:  assert.NoError,
		},
		{
			name:     "flask",
			file:     "testdata/poetry_flask.lock",
			wantLibs: poetryFlask,
			wantDeps: poetryFlaskDeps,
			wantErr:  assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.Open(tt.file)
			require.NoError(t, err)
			defer f.Close()

			p := &Parser{}
			gotLibs, gotDeps, err := p.Parse(nil, f)
			if !tt.wantErr(t, err, fmt.Sprintf("Parse(%v)", tt.file)) {
				return
			}
			ids := map[string]string{}
			for i := range gotLibs {
				ids[gotLibs[i].ID] = gotLibs[i].Name + "@" + gotLibs[i].Version
				gotLibs[i].ID = ids[gotLibs[i].ID]
				gotLibs[i].Condition = ""
				gotLibs[i].Scope = ""
				gotLibs[i].Verification = ""
				gotLibs[i].DeclaredIntegrity = ""
				gotLibs[i].Diagnostics = nil
				gotLibs[i].Locations = nil
				gotLibs[i].Source = ""
			}
			for i := range gotDeps {
				gotDeps[i].ID = ids[gotDeps[i].ID]
				for j, ref := range gotDeps[i].DependsOn {
					if v := ids[ref]; v != "" {
						gotDeps[i].DependsOn[j] = v
					}
				}
				gotDeps[i].Requirements = nil
			}
			assert.Equalf(t, tt.wantLibs, gotLibs, "Parse(%v)", tt.file)
			assert.Equalf(t, tt.wantDeps, gotDeps, "Parse(%v)", tt.file)
		})
	}
}

func TestParseDependency(t *testing.T) {
	tests := []struct {
		name         string
		packageName  string
		versionRange interface{}
		libsVersions map[string][]poetryIdent
		want         string
		wantErr      string
	}{
		{
			name:         "handle package name",
			packageName:  "Test_project.Name",
			versionRange: "*",
			libsVersions: map[string][]poetryIdent{
				"test-project-name": {{Version: "1.0.0", ID: poetryNativeID("test-project-name", "1.0.0", "")}},
			},
			want: poetryNativeID("test-project-name", "1.0.0", ""),
		},
		{
			name:         "version range as string",
			packageName:  "test",
			versionRange: ">=1.0.0",
			libsVersions: map[string][]poetryIdent{
				"test": {{Version: "2.0.0", ID: poetryNativeID("test", "2.0.0", "")}},
			},
			want: poetryNativeID("test", "2.0.0", ""),
		},
		{
			name:         "version range == *",
			packageName:  "test",
			versionRange: "*",
			libsVersions: map[string][]poetryIdent{
				"test": {{Version: "3.0.0", ID: poetryNativeID("test", "3.0.0", "")}},
			},
			want: poetryNativeID("test", "3.0.0", ""),
		},
		{
			name:        "version range as json",
			packageName: "test",
			versionRange: map[string]interface{}{
				"version": ">=4.8.3",
				"markers": "python_version < \"3.8\"",
			},
			libsVersions: map[string][]poetryIdent{
				"test": {{Version: "5.0.0", ID: poetryNativeID("test", "5.0.0", "")}},
			},
			want: poetryNativeID("test", "5.0.0", ""),
		},
		{
			name:         "libsVersions doesn't contain required version",
			packageName:  "test",
			versionRange: ">=1.0.0",
			libsVersions: map[string][]poetryIdent{},
			wantErr:      "no version found",
		},
		{
			name:         "ambiguous same-name not picked by order",
			packageName:  "amb",
			versionRange: ">=1.0",
			libsVersions: map[string][]poetryIdent{
				"amb": {
					{Version: "1.0.0", ID: poetryNativeID("amb", "1.0.0", "")},
					{Version: "2.0.0", ID: poetryNativeID("amb", "2.0.0", "")},
				},
			},
			wantErr: "ambiguous locked versions",
		},
		{
			name:         "same version different source not uniquely resolved",
			packageName:  "foo",
			versionRange: "*",
			libsVersions: map[string][]poetryIdent{
				"foo": {
					{Version: "1.0.0", Source: "https://a.example/repo.git#abcdef", ID: poetryNativeID("foo", "1.0.0", "https://a.example/repo.git#abcdef")},
					{Version: "1.0.0", Source: "https://b.example/repo.git#abcdef", ID: poetryNativeID("foo", "1.0.0", "https://b.example/repo.git#abcdef")},
				},
			},
			wantErr: "ambiguous locked versions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDependency(tt.packageName, tt.versionRange, tt.libsVersions)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
