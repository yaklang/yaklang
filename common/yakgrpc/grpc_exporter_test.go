package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"testing"
)

func TestGRPCMUSTPASS_CSV_Exporter(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx := context.Background()

	stream, err := client.ExtractDataToFile(ctx)
	require.NoError(t, err)

	// Send config
	err = stream.Send(&ypb.ExtractDataToFileRequest{
		JsonOutput:      false,
		CSVOutput:       true,
		DirName:         "",
		Data:            nil,
		FileNamePattern: "",
		Finished:        false,
	})
	require.NoError(t, err)

	// Send data
	err = stream.Send(&ypb.ExtractDataToFileRequest{
		Data: map[string]*ypb.ExtractableData{
			"test": {StringValue: `{"message":"Hello, this is a more complex JSON response!","id":12345,"active":true,"details":{"name":"SampleService","version":"1.0.0"}}`},
		},
	})
	require.NoError(t, err)

	// Indicate that we are done sending data
	err = stream.Send(&ypb.ExtractDataToFileRequest{
		Finished: true,
	})
	require.NoError(t, err)

	// Receive response
	resp, err := stream.Recv()
	require.NoError(t, err)

	// Check the response
	require.Contains(t, resp.FilePath, ".csv")
	content, err := os.ReadFile(resp.FilePath)
	require.NoError(t, err)
	require.Contains(t, string(content), "\ufefftest\n\"{\"\"message\"\":\"\"Hello, this is a more complex JSON response!\"\",\"\"id\"\":12345,\"\"active\"\":true,\"\"details\"\":{\"\"name\"\":\"\"SampleService\"\",\"\"version\"\":\"\"1.0.0\"\"}}\"\n")
	err = os.Remove(resp.FilePath)
	require.NoError(t, err)
}
