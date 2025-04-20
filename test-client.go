package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/transport/http"
)

func main() {
	// Create HTTP transport
	transport := http.NewHTTPClientTransport("/mcp")
	transport.WithBaseURL("http://localhost:8080")
	transport.WithEventsEndpoint("/events")
	
	// Create a new MCP client
	mcpClient := mcp.NewClient(transport)

	// Initialize the client
	err = mcpClient.Initialize(context.Background())
	if err != nil {
		log.Fatalf("Failed to initialize MCP client: %v", err)
	}

	// List available tools
	tools, err := mcpClient.ListTools(context.Background())
	if err != nil {
		log.Fatalf("Failed to list tools: %v", err)
	}

	fmt.Println("Available tools:")
	for _, tool := range tools {
		fmt.Printf("- %s: %s\n", tool.Name, tool.Description)
	}

	// Test listSchemas tool
	fmt.Println("\nTesting listSchemas tool...")
	result, err := mcpClient.CallTool(context.Background(), "listSchemas", nil)
	if err != nil {
		log.Fatalf("Failed to call listSchemas tool: %v", err)
	}
	fmt.Printf("Result: %s\n", result.Content[0].Text)

	// Test executeQuery tool with broadcast
	fmt.Println("\nTesting executeQuery tool with broadcast...")
	args := map[string]interface{}{
		"query":     "SELECT current_database(), current_user",
		"schema":    "public",
		"broadcast": true,
		"eventName": "test_query_result",
	}
	result, err = mcpClient.CallTool(context.Background(), "executeQuery", args)
	if err != nil {
		log.Fatalf("Failed to call executeQuery tool: %v", err)
	}
	fmt.Printf("Result: %s\n", result.Content[0].Text)

	// Listen for events
	fmt.Println("\nListening for events (for 5 seconds)...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events := mcpClient.Events()
	go func() {
		for {
			select {
			case event := <-events:
				fmt.Printf("Received event: %s - %v\n", event.Name, event.Data)
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for events to be received
	time.Sleep(5 * time.Second)
	fmt.Println("Test complete!")
}
