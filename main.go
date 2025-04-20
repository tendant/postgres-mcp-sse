package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/tendant/postgres-mcp-sse/internal/db"
	"github.com/tendant/postgres-mcp-sse/internal/server"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Create a custom Hub type for backward compatibility with existing handlers
type Hub struct {
	broadcast chan server.Event
}

func NewHub() *Hub {
	return &Hub{
		broadcast: make(chan server.Event),
	}
}

func (h *Hub) Run() {
	// Implementation for backward compatibility
}

// Broadcast returns the broadcast channel for sending events
func (h *Hub) Broadcast() chan<- server.Event {
	return h.broadcast
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Simple SSE implementation for backward compatibility
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Keep the connection open
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-h.broadcast:
			data, _ := json.Marshal(event.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Name, data)
			flusher.Flush()
		}
	}
}

func main() {
	// Initialize Postgres connection
	dbConn, err := db.InitPostgres("postgres://user:password@postgres:5432/mydb?sslmode=disable")
	if err != nil {
		log.Fatalf("DB error: %v", err)
	}
	defer dbConn.Close()

	// Create a custom Hub for backward compatibility with existing handlers
	hub := NewHub()
	go hub.Run()

	// Create a new MCP server with logging and recovery middleware
	mcpSrv := mcpserver.NewMCPServer(
		"Postgres MCP Server",
		"1.0.0",
		mcpserver.WithResourceCapabilities(true, true),
		mcpserver.WithLogging(),
		mcpserver.WithRecovery(),
	)

	// Create SSE server for MCP
	sseServer := mcpserver.NewSSEServer(mcpSrv)

	// Create HTTP server mux
	mux := http.NewServeMux()

	// Set up SSE events endpoint for backward compatibility
	mux.HandleFunc("/events", hub.ServeHTTP)

	// Set up MCP server HTTP handler
	mux.HandleFunc("/mcp", sseServer.ServeHTTP)

	// Set up database query handlers
	mux.HandleFunc("/query/execute", server.ExecuteQueryHandler(dbConn, hub))
	mux.HandleFunc("/schema/full", server.FullTableSchemaHandler(dbConn))
	mux.HandleFunc("/schema/tables", server.ListTablesHandler(dbConn))
	mux.HandleFunc("/schema/describe", server.DescribeTableHandler(dbConn))
	mux.HandleFunc("/schema/sample", server.SampleRowsHandler(dbConn))
	mux.HandleFunc("/schema/foreign_keys", server.ForeignKeysHandler(dbConn))
	mux.HandleFunc("/schema/list_schemas", server.ListSchemasHandler(dbConn))

	// Add database query tool to MCP server
	queryTool := mcp.NewTool("executeQuery",
		mcp.WithDescription("Execute a SQL query against the database"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("SQL query to execute"),
		),
		mcp.WithString("schema",
			mcp.Description("Database schema to use"),
			mcp.DefaultString("public"),
		),
	)

	// Add the query tool handler
	mcpSrv.AddTool(queryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := request.Params.Arguments["query"].(string)
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}
		
		// Execute the query
		result, err := executeQuery(dbConn, schema, query)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Query error: %v", err)), nil
		}
		
		// Convert result to JSON string
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("JSON encoding error: %v", err)), nil
		}
		
		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Start the HTTP server
	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// executeQuery executes a SQL query and returns the results
func executeQuery(db *sql.DB, schema, query string) (map[string]interface{}, error) {
	// Set the schema
	_, err := db.Exec(fmt.Sprintf("SET search_path TO %s", schema))
	if err != nil {
		return nil, fmt.Errorf("failed to set schema: %w", err)
	}

	// Execute the query
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	// Get column names
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Process results
	var results []map[string]interface{}
	for rows.Next() {
		columnVals := make([]interface{}, len(cols))
		columnPtrs := make([]interface{}, len(cols))
		for i := range columnVals {
			columnPtrs[i] = &columnVals[i]
		}
		
		if err := rows.Scan(columnPtrs...); err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		
		rowMap := make(map[string]interface{})
		for i, col := range cols {
			rowMap[col] = columnVals[i]
		}
		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return map[string]interface{}{
		"columns": cols,
		"rows":    results,
	}, nil
}
