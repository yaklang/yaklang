package ssadb

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestSaveType(t *testing.T) {
	// Initialize the database connection

	kind := 1
	str := "test string"
	extra := "extra information" + uuid.NewString()

	// Save the type
	id := SaveType(kind, str, extra)
	defer DeleteType(id) // Clear data after test

	// Retrieve the type
	retrievedKind, retrievedStr, retrievedExtra, err := GetType(id)
	assert.NoError(t, err)
	assert.Equal(t, kind, retrievedKind)
	assert.Equal(t, str, retrievedStr)
	assert.Equal(t, extra, retrievedExtra)
}

func TestSaveTypeReuse(t *testing.T) {

	kind := 2
	str := "another test string"
	extra := "more extra information" + uuid.NewString()

	// Save the type
	id1 := SaveType(kind, str, extra)
	defer DeleteType(id1) // Clear data after test

	// Save the same type again
	id2 := SaveType(kind, str, extra)

	// Ensure the IDs are the same, indicating reuse
	assert.Equal(t, id1, id2)

	// Retrieve the type
	retrievedKind, retrievedStr, retrievedExtra, err := GetType(id1)
	assert.NoError(t, err)
	assert.Equal(t, kind, retrievedKind)
	assert.Equal(t, str, retrievedStr)
	assert.Equal(t, extra, retrievedExtra)
}

func TestSaveTypeConcurrent(t *testing.T) {
	kind := 3
	str := "concurrent test string"
	extra := "concurrent extra information" + uuid.NewString()

	var id1, id2 int

	// Save the type concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		id1 = SaveType(kind, str, extra)
	}()
	go func() {
		defer wg.Done()
		id2 = SaveType(kind, str, extra)
	}()

	wg.Wait()

	// defer DeleteType(id1) // Clear data after test

	// Ensure the IDs are the same, indicating reuse
	assert.Equal(t, id1, id2)

	// Retrieve the type
	retrievedKind, retrievedStr, retrievedExtra, err := GetType(id1)
	assert.NoError(t, err)
	assert.Equal(t, kind, retrievedKind)
	assert.Equal(t, str, retrievedStr)
	assert.Equal(t, extra, retrievedExtra)
}
