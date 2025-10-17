package ssadb

// import (
// 	"sync"
// 	"testing"

// 	"github.com/google/uuid"
// 	"github.com/stretchr/testify/require"
// )

// func TestSaveType(t *testing.T) {
// 	// Initialize the database connection

// 	progName := uuid.NewString()
// 	kind := 1
// 	str := "test string"
// 	extra := "extra information" + uuid.NewString()

// 	// Save the type
// 	id := SaveType(kind, str, extra, progName)
// 	defer DeleteType(id) // Clear data after test

// 	// Retrieve the type
// 	retrievedKind, retrievedStr, retrievedExtra, err := GetType(id)
// 	require.NoError(t, err)
// 	require.Equal(t, kind, retrievedKind)
// 	require.Equal(t, str, retrievedStr)
// 	require.Equal(t, extra, retrievedExtra)
// }

// func TestSaveTypeReuse(t *testing.T) {

// 	kind := 2
// 	str := "another test string"
// 	extra := "more extra information" + uuid.NewString()
// 	progName := uuid.NewString()

// 	// Save the type
// 	id1 := SaveType(kind, str, extra, progName)
// 	defer DeleteType(id1) // Clear data after test

// 	// Save the same type again
// 	id2 := SaveType(kind, str, extra, progName)

// 	// Ensure the IDs are the same, indicating reuse
// 	require.Equal(t, id1, id2)

// 	// Retrieve the type
// 	retrievedKind, retrievedStr, retrievedExtra, err := GetType(id1)
// 	require.NoError(t, err)
// 	require.Equal(t, kind, retrievedKind)
// 	require.Equal(t, str, retrievedStr)
// 	require.Equal(t, extra, retrievedExtra)
// }

// func TestSaveTypeConcurrent(t *testing.T) {
// 	kind := 3
// 	str := "concurrent test string"
// 	extra := "concurrent extra information" + uuid.NewString()
// 	progName := uuid.NewString()
// 	var id1, id2 int

// 	// Save the type concurrently
// 	var wg sync.WaitGroup
// 	wg.Add(2)

// 	go func() {
// 		defer wg.Done()
// 		id1 = SaveType(kind, str, extra, progName)
// 	}()
// 	go func() {
// 		defer wg.Done()
// 		id2 = SaveType(kind, str, extra, progName)
// 	}()

// 	wg.Wait()

// 	// defer DeleteType(id1) // Clear data after test

// 	// Ensure the IDs are the same, indicating reuse
// 	require.Equal(t, id1, id2)

// 	// Retrieve the type
// 	retrievedKind, retrievedStr, retrievedExtra, err := GetType(id1)
// 	require.NoError(t, err)
// 	require.Equal(t, kind, retrievedKind)
// 	require.Equal(t, str, retrievedStr)
// 	require.Equal(t, extra, retrievedExtra)
// }
