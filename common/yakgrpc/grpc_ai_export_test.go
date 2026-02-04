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

func TestGRPCMUSTPASS_ExportAILogsBySessionID(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	require.NotNil(t, db)

	// Generate random IDs
	sessionID := uuid.NewString()
	coordinatorID1 := uuid.NewString()
	coordinatorID2 := uuid.NewString()

	// Prepare test data - Create events with sessionID first
	event1 := &schema.AiOutputEvent{
		CoordinatorId: coordinatorID1,
		SessionId:     sessionID,
		Type:          schema.EVENT_TYPE_STREAM,
		EventUUID:     uuid.NewString(),
		Content:       []byte("test content 1"),
	}
	require.NoError(t, db.Create(event1).Error)

	event2 := &schema.AiOutputEvent{
		CoordinatorId: coordinatorID2,
		SessionId:     sessionID,
		Type:          schema.EVENT_TYPE_STRUCTURED,
		EventUUID:     uuid.NewString(),
		Content:       []byte("test content 2"),
	}
	require.NoError(t, db.Create(event2).Error)

	// Create checkpoints associated with the coordinatorIDs
	checkpoint1 := &schema.AiCheckpoint{
		CoordinatorUuid:    coordinatorID1,
		Seq:                1,
		Type:               schema.AiCheckpointType_AIInteractive,
		RequestQuotedJson:  "{}",
		ResponseQuotedJson: "{}",
	}
	require.NoError(t, db.Create(checkpoint1).Error)

	checkpoint2 := &schema.AiCheckpoint{
		CoordinatorUuid:    coordinatorID2,
		Seq:                1,
		Type:               schema.AiCheckpointType_AIInteractive,
		RequestQuotedJson:  "{}",
		ResponseQuotedJson: "{}",
	}
	require.NoError(t, db.Create(checkpoint2).Error)

	// Create memory associated with sessionID
	memory := &schema.AIMemoryEntity{
		MemoryID:  uuid.NewString(),
		SessionID: sessionID,
		Content:   "test memory content for session",
	}
	require.NoError(t, db.Create(memory).Error)

	// Create timeline
	timeline := &schema.AIAgentRuntime{
		PersistentSession: sessionID,
		Uuid:              uuid.NewString(),
		Name:              "test agent by session",
	}
	require.NoError(t, db.Create(timeline).Error)

	// Cleanup function
	defer func() {
		db.Where("session_id = ?", sessionID).Delete(&schema.AiOutputEvent{})
		db.Where("coordinator_uuid IN (?)", []string{coordinatorID1, coordinatorID2}).Delete(&schema.AiCheckpoint{})
		db.Where("session_id = ?", sessionID).Delete(&schema.AIMemoryEntity{})
		db.Where("persistent_session = ?", sessionID).Delete(&schema.AIAgentRuntime{})
	}()

	// Initialize Server
	server := &Server{}

	// Test ExportAILogs using ONLY sessionID (no coordinatorIDs, no memoryID)
	req := &ypb.ExportAILogsRequest{
		SessionID:       sessionID,
		ExportDataTypes: []string{"checkpoints", "output_event", "memory", "timeline"},
	}

	resp, err := server.ExportAILogs(context.Background(), req)
	require.NoError(t, err)
	require.NotEmpty(t, resp.FilePath)

	// Verify Zip Content
	verifyZipContentBySessionID(t, resp.FilePath, coordinatorID1, coordinatorID2)

	// Clean up zip file
	os.Remove(resp.FilePath)
}

func verifyZipContentBySessionID(t *testing.T, zipPath string, coordinatorID1, coordinatorID2 string) {
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

		// Basic check that data is a list and not empty
		arr, ok := data.([]interface{})
		if assert.True(t, ok, "File %s content should be a JSON array", f.Name) {
			assert.NotEmpty(t, arr, "File %s should not be empty", f.Name)

			// For checkpoints, verify we got both coordinatorIDs
			if f.Name == "AICheckpoints.json" {
				assert.GreaterOrEqual(t, len(arr), 2, "Should have at least 2 checkpoints")
			}

			// For output_event, verify we got both events
			if f.Name == "AIOutputEvent.json" {
				assert.GreaterOrEqual(t, len(arr), 2, "Should have at least 2 output events")
			}
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
