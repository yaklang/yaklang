package sfreport

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

// The report carries the language the scan already decided for the program; it
// must not re-derive it from a file extension.
func TestReportUsesSchemaRiskLanguage(t *testing.T) {
	report := NewReport(IRifyReportType)
	metadata := &schema.SSARisk{
		CodeSourceUrl: "/program/main.go",
		Line:          1,
		Language:      "java",
	}
	risk, _ := NewRisk(metadata, report)
	require.Equal(t, "java", risk.Language)
}

func TestSarifRegionCarriesRiskLanguage(t *testing.T) {
	language := "golang"
	location := locationFromRisk(&schema.SSARisk{
		CodeSourceUrl: "/program/main.go",
		Line:          3,
		Language:      language,
	})
	require.NotNil(t, location)
	region := location.PhysicalLocation.Region
	require.NotNil(t, region)
	require.Equal(t, language, *region.SourceLanguage)
}

func TestSarifRegionOmitsEmptyLanguage(t *testing.T) {
	location := locationFromRisk(&schema.SSARisk{CodeSourceUrl: "/program/main.go", Line: 3})
	require.NotNil(t, location)
	require.Nil(t, location.PhysicalLocation.Region.SourceLanguage)
}
