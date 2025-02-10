package main

import (
	"log"
	"time"

	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/mcp/transport/http"
)

// TimeArgs defines the arguments for the time tool
type TimeArgs struct {
	Format string `json:"format" jsonschema:"description=The time format to use"`
}

func main() {
	// Create an HTTP transport that listens on /mcp endpoint
	transport := http.NewHTTPTransport("/mcp").WithAddr(":8081")

	// Create a new server with the transport
	server := mcp.NewServer(transport, mcp.WithName("mcp-golang-stateless-http-example"), mcp.WithVersion("0.0.1"))

	// Register a simple tool
	err := server.RegisterTool("time", "Returns the current time in the specified format", func(args TimeArgs) (*mcp.ToolResponse, error) {
		format := args.Format
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		log.Println("Received time request with format:", format)
		return mcp.NewToolResponse(mcp.NewTextContent(time.Now().Format(format))), nil
	})
	if err != nil {
		panic(err)
	}

	// Start the server
	log.Println("Starting HTTP server on :8081...")
	server.Serve()
}
