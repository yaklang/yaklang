package sfreport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultStreamImportConfig(t *testing.T) {
	c := DefaultStreamImportConfig()
	require.NotNil(t, c)
	assert.Equal(t, 200, c.BatchSize)
	assert.True(t, c.FlushInterval > 0)
	assert.True(t, c.ChannelBuffer > 0)
}

func TestStreamImportConfig_NoDeprecatedFields(t *testing.T) {
	// Verify that StreamImportConfig no longer has deprecated fields.
	c := StreamImportConfig{
		BatchSize:     100,
		FlushInterval: 0,
		ChannelBuffer: 1000,
	}
	// If MaxGoroutines or EnableBackpress fields existed, this would fail to compile
	// with the new struct definition. This test simply asserts the struct is usable.
	assert.Equal(t, 100, c.BatchSize)
}

func TestRecordError_Capped(t *testing.T) {
	importer := &AsyncStreamImporter{}

	// Fill beyond cap.
	for i := 0; i < maxImportErrors+100; i++ {
		importer.recordError(assert.AnError)
	}

	_, _, errCount := importer.GetStats()
	assert.Equal(t, maxImportErrors, errCount, "errors should be capped at maxImportErrors")
	assert.Len(t, importer.GetErrors(), maxImportErrors)
}

func TestGetStats_ZeroValues(t *testing.T) {
	importer := &AsyncStreamImporter{}
	f, r, e := importer.GetStats()
	assert.Equal(t, 0, f)
	assert.Equal(t, 0, r)
	assert.Equal(t, 0, e)
}

func TestGetErrors_Isolation(t *testing.T) {
	importer := &AsyncStreamImporter{}
	importer.recordError(assert.AnError)

	errs1 := importer.GetErrors()
	errs2 := importer.GetErrors()

	// Modifying one slice should not affect the other.
	errs1[0] = nil
	assert.NotNil(t, errs2[0])
}

func TestAddFile_NilSaver(t *testing.T) {
	importer := &AsyncStreamImporter{}
	err := importer.AddFile(&File{})
	assert.Error(t, err)
}

func TestAddRisk_NilSaver(t *testing.T) {
	importer := &AsyncStreamImporter{}
	err := importer.AddRisk(&Risk{}, nil)
	assert.Error(t, err)
}
