package stream_parser

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStructuredProjectionMatchesNodeSemantics(t *testing.T) {
	wire := []byte{0x80, 0xff, 0x01, 0x02, 'a', 'b'}
	var fields []tlsCertificateField
	for _, typ := range []string{"int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "string", "bytes", "raw"} {
		for _, endian := range []string{"big", "little"} {
			fields = append(fields, tlsCertificateField{Name: typ + endian, Type: typ, Start: 0, End: 2, Endian: endian})
		}
	}
	fields = append(fields,
		tlsCertificateField{Name: "Little Parent", Start: 0, End: 4, Endian: "little", Children: []tlsCertificateField{
			{Name: "Inherited", Type: "uint16", Start: 2, End: 4},
			{Name: "Overridden", Type: "uint16", Start: 2, End: 4, Endian: "big"},
		}},
		tlsCertificateField{Name: "Empty List", List: true},
		tlsCertificateField{Name: "Absent"},
		tlsCertificateField{Name: "Empty Raw", Type: "raw"},
		tlsCertificateField{Name: "Duplicates", Start: 0, End: 4, Children: []tlsCertificateField{
			{Name: "Value", Type: "uint8", Start: 0, End: 1},
			{Name: "Value", Type: "uint8", Start: 2, End: 3},
		}},
		tlsCertificateField{Name: "List", List: true, Start: 0, End: 4, Children: []tlsCertificateField{
			{Name: "Absent"},
			{Name: "Value", Type: "uint8", Start: 0, End: 1},
			{Name: "Value", Type: "uint8", Start: 2, End: 3},
		}},
	)
	node := positionedResultNode(wire, 0)
	require.NoError(t, buildExactByteFieldTree(node, fields, nil, 0, uint64(len(wire)*8), "projection", "big"))
	value, err := projectStructuredFields(wire, fields, false, "big")
	require.NoError(t, err)
	require.Equal(t, NodeToMap(node), value)
	clear(wire)
	require.Equal(t, NodeToMap(node), value)
}

func TestStructuredProjectionRejectsUnsupportedFields(t *testing.T) {
	for _, field := range []tlsCertificateField{
		{Name: "Negative", Start: -1}, {Name: "Reversed", Start: 1}, {Name: "Outside", End: 2},
		{Name: "Endian", Endian: "middle"}, {Name: "Reference", Type: "SomeType"},
		{Name: "Terminal Children", Type: "uint8", Children: []tlsCertificateField{{}}},
	} {
		t.Run(field.Name, func(t *testing.T) {
			value, err := projectStructuredFields([]byte{1}, []tlsCertificateField{field}, false, "big")
			require.Error(t, err)
			require.Nil(t, value)
		})
	}
}

func TestStructuredDecoderFixtureEquivalence(t *testing.T) {
	for _, family := range []struct {
		function string
		fixtures map[string][]byte
		decode   func([]byte, string) ([]tlsCertificateField, map[string]any, error)
	}{
		{"parseMemcachedFields", memcachedFieldsTestFixtures(), decodeMemcachedFields},
		{"parseCassandraFields", cassandraFieldsTestFixtures(), decodeCassandraFields},
		{"parseTNSFields", tnsFieldsTestFixtures(), decodeTNSFields},
		{"parseTDSFields", tdsFieldsTestFixtures(), decodeTDSFields},
		{"parseMySQLFields", mysqlFieldsTestFixtures(), decodeMySQLFields},
		{"parsePostgreSQLFields", postgresqlFieldsTestFixtures(), decodePostgreSQLFields},
	} {
		for profile, wire := range family.fixtures {
			t.Run(family.function+"/"+profile, func(t *testing.T) {
				fields, metadata, err := family.decode(wire, profile)
				require.NoError(t, err)
				node := positionedResultNode(wire, 0)
				endian := "big"
				if family.function == "parseMySQLFields" {
					endian = "little"
				}
				require.NoError(t, buildExactByteFieldTree(node, fields, metadata, 0, uint64(len(wire)*8), profile, endian))
				decoder := StructuredDecoderForProgram(fmt.Sprintf("err = %s(%q)\nif err != nil { panic(err) }\n", family.function, profile))
				require.NotNil(t, decoder)
				value, info, err := decoder(wire)
				require.NoError(t, err)
				require.Equal(t, metadata, info)
				require.Equal(t, NodeToMap(node), value)
			})
		}
	}
}
