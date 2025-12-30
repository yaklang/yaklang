package yakgrpc

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_ExportAILogs(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	require.NotNil(t, db)

	// Generate random IDs
	sessionID := uuid.NewString()
	coordinatorID := uuid.NewString()
	memoryID := uuid.NewString()     // This will be used as session_id in AIMemoryEntity query
	testMemoryID := uuid.NewString() // This is the unique ID for the memory entity itself

	// Prepare test data
	checkpoint := &schema.AiCheckpoint{
		CoordinatorUuid:    coordinatorID,
		Seq:                1,
		Type:               schema.AiCheckpointType_AIInteractive,
		RequestQuotedJson:  "{}",
		ResponseQuotedJson: "{}",
	}
	require.NoError(t, db.Create(checkpoint).Error)

	event := &schema.AiOutputEvent{
		CoordinatorId: coordinatorID,
		Type:          schema.EVENT_TYPE_STREAM,
		EventUUID:     uuid.NewString(),
		Content:       []byte("test content"),
	}
	require.NoError(t, db.Create(event).Error)

	memory := &schema.AIMemoryEntity{
		MemoryID:  testMemoryID,
		SessionID: memoryID, // Note: ExportAILogs queries by session_id using the passed memoryID
		Content:   "test memory content",
	}
	require.NoError(t, db.Create(memory).Error)

	timeline := &schema.AIAgentRuntime{
		PersistentSession: sessionID,
		Uuid:              uuid.NewString(),
		Name:              "test agent",
	}
	require.NoError(t, db.Create(timeline).Error)

	// Cleanup function
	defer func() {
		db.Where("coordinator_uuid = ?", coordinatorID).Delete(&schema.AiCheckpoint{})
		db.Where("coordinator_id = ?", coordinatorID).Delete(&schema.AiOutputEvent{})
		db.Where("session_id = ?", memoryID).Delete(&schema.AIMemoryEntity{})
		db.Where("persistent_session = ?", sessionID).Delete(&schema.AIAgentRuntime{})
	}()

	// Initialize Server
	server := &Server{} // Assuming Server struct has GetProjectDatabase method that uses consts.GetGormProjectDatabase or similar mechanism,
	// but looking at source code:
	// func (s *Server) ExportAILogs(...) {
	//     db := s.GetProjectDatabase()
	// ...
	// So we need to make sure s.GetProjectDatabase() returns the db we used.
	// yakgrpc/server.go usually implements this. Since we are in the same package, we might not need to mock if Server uses the global DB getter by default or if we can set it.
	// Checking previous test files: grpc_ai_event_test.go uses CreateOrUpdateAIOutputEvent which uses db directly, but it tests Server methods by creating a client.
	// Here we want to call Server method directly or via client.
	// Direct call is easier if Server is simple.
	// Let's check how Server gets DB.

	// In yakgrpc/server.go (speculated), GetProjectDatabase usually calls consts.GetGormProjectDatabase().
	// If so, &Server{} should work fine.

	// Test ExportAILogs
	req := &ypb.ExportAILogsRequest{
		SessionID:       sessionID,
		CoordinatorIDs:  []string{coordinatorID},
		MemoryID:        memoryID,
		ExportDataTypes: []string{"checkpoints", "output_event", "memory", "timeline"},
	}

	resp, err := server.ExportAILogs(context.Background(), req)
	require.NoError(t, err)
	require.NotEmpty(t, resp.FilePath)

	// Verify Zip Content
	verifyZipContent(t, resp.FilePath)

	// Clean up zip file
	os.Remove(resp.FilePath)
}

func verifyZipContent(t *testing.T, zipPath string) {
	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer r.Close()

	filesFound := make(map[string]bool)
	for _, f := range r.File {
		filesFound[f.Name] = true
		rc, err := f.Open()
		require.NoError(t, err)

		var data interface{}
		err = json.NewDecoder(rc).Decode(&data)
		require.NoError(t, err)
		rc.Close()

		// Basic check that data is a list and not empty (since we inserted 1 record for each)
		arr, ok := data.([]interface{})
		if assert.True(t, ok, "File %s content should be a JSON array", f.Name) {
			assert.NotEmpty(t, arr, "File %s should not be empty", f.Name)
		}
	}

	expectedFiles := []string{
		"AICheckpoints.json",
		"AIOutputEvent.json",
		"memory.json",
		"timeline.json",
	}

	for _, name := range expectedFiles {
		assert.True(t, filesFound[name], "Expected file %s not found in zip", name)
	}
}
