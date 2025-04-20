package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
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
	dbConn, err := db.InitPostgres("postgres://postgres:pwd@localhost:5432/postgres?sslmode=disable")
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

	// Create a simple HTTP handler for MCP
	mcpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Only accept POST requests for JSON-RPC
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read the request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}

		// Parse the JSON-RPC request
		var request map[string]interface{}
		if err := json.Unmarshal(body, &request); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Handle the request based on the method
		method, _ := request["method"].(string)
		params, _ := request["params"].(map[string]interface{})
		id := request["id"]

		var result interface{}

		switch method {
		case "initialize":
			// Handle initialize request
			result = map[string]interface{}{
				"server_info": map[string]interface{}{
					"name":    "Postgres MCP Server",
					"version": "1.0.0",
				},
				"protocol_version": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{
						"list_changed": false,
					},
				},
			}
		case "tools/list":
			// Get all tools from the MCP server
			tools := []map[string]interface{}{
				{
					"name":        "executeQuery",
					"description": "Execute a SQL query against the database",
				},
				{
					"name":        "getFullTableSchema",
					"description": "Get full schema information for a table",
				},
				{
					"name":        "listTables",
					"description": "List all tables in a schema",
				},
				{
					"name":        "describeTable",
					"description": "Get column information for a table",
				},
				{
					"name":        "sampleRows",
					"description": "Get sample rows from a table",
				},
				{
					"name":        "getForeignKeys",
					"description": "Get foreign key relationships for a table",
				},
				{
					"name":        "listSchemas",
					"description": "List all schemas in the database",
				},
			}
			result = map[string]interface{}{
				"tools": tools,
			}
		case "tools/call":
			// Handle tool calls
			toolName, _ := params["name"].(string)
			toolArgs, _ := params["arguments"].(map[string]interface{})

			// Process the tool call based on the tool name
			switch toolName {
			case "executeQuery":
				query, _ := toolArgs["query"].(string)
				schema, ok := toolArgs["schema"].(string)
				if !ok {
					schema = "public"
				}
				
				// Check if we should broadcast the results
				broadcast, _ := toolArgs["broadcast"].(bool)
				eventName, _ := toolArgs["eventName"].(string)
				if eventName == "" {
					eventName = "query_result"
				}

				// Create a request body for the ExecuteQueryHandler
				reqBody, _ := json.Marshal(server.QueryRequest{
					Schema:    schema,
					Query:     query,
					Broadcast: broadcast,
					EventName: eventName,
				})

				// Create a mock HTTP request to reuse the existing handler logic
				req, _ := http.NewRequest("POST", "/query/execute", bytes.NewBuffer(reqBody))
				req.Header.Set("Content-Type", "application/json")
				rw := newResponseRecorder()

				// Execute the query using the existing handler
				server.ExecuteQueryHandler(dbConn, hub)(rw, req)

				if rw.statusCode != http.StatusOK {
					result = map[string]interface{}{
						"content": []map[string]interface{}{
							{
								"type": "text",
								"text": fmt.Sprintf("Error: %s", rw.body.String()),
							},
						},
					}
				} else {
					result = map[string]interface{}{
						"content": []map[string]interface{}{
							{
								"type": "text",
								"text": rw.body.String(),
							},
						},
					}
				}
			case "listSchemas":
				// Create a mock HTTP request to reuse the existing handler logic
				req, _ := http.NewRequest("GET", "/schema/list_schemas", nil)
				rw := newResponseRecorder()
				server.ListSchemasHandler(dbConn)(rw, req)
				result = map[string]interface{}{
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": rw.body.String(),
						},
					},
				}
			case "listTables":
				schema, ok := toolArgs["schema"].(string)
				if !ok {
					schema = "public"
				}
				req, _ := http.NewRequest("GET", fmt.Sprintf("/schema/tables?schema=%s", schema), nil)
				rw := newResponseRecorder()
				server.ListTablesHandler(dbConn)(rw, req)
				result = map[string]interface{}{
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": rw.body.String(),
						},
					},
				}
			default:
				result = map[string]interface{}{
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": fmt.Sprintf("Tool '%s' not implemented yet", toolName),
						},
					},
				}
			}
		default:
			// Unknown method
			http.Error(w, fmt.Sprintf("Unknown method: %s", method), http.StatusBadRequest)
			return
		}

		// Create the JSON-RPC response
		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result":  result,
		}

		// Send the response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Create HTTP server mux
	mux := http.NewServeMux()

	// Set up SSE events endpoint for backward compatibility
	mux.HandleFunc("/events", hub.ServeHTTP)

	// Set up MCP server HTTP handler
	mux.Handle("/mcp", mcpHandler)

	// Set up database query handlers (keep for backward compatibility)
	mux.HandleFunc("/query/execute", server.ExecuteQueryHandler(dbConn, hub))
	mux.HandleFunc("/schema/full", server.FullTableSchemaHandler(dbConn))
	mux.HandleFunc("/schema/tables", server.ListTablesHandler(dbConn))
	mux.HandleFunc("/schema/describe", server.DescribeTableHandler(dbConn))
	mux.HandleFunc("/schema/sample", server.SampleRowsHandler(dbConn))
	mux.HandleFunc("/schema/foreign_keys", server.ForeignKeysHandler(dbConn))
	mux.HandleFunc("/schema/list_schemas", server.ListSchemasHandler(dbConn))

	// Add database query tools to MCP server

	// 1. Execute Query Tool
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

	// 2. Full Table Schema Tool
	fullSchemaToolTool := mcp.NewTool("getFullTableSchema",
		mcp.WithDescription("Get full schema information for a table including columns, foreign keys, and sample data"),
		mcp.WithString("table",
			mcp.Required(),
			mcp.Description("Table name"),
		),
		mcp.WithString("schema",
			mcp.Description("Database schema name"),
			mcp.DefaultString("public"),
		),
	)

	// 3. List Tables Tool
	listTablesToolTool := mcp.NewTool("listTables",
		mcp.WithDescription("List all tables in a schema"),
		mcp.WithString("schema",
			mcp.Description("Database schema name"),
			mcp.DefaultString("public"),
		),
	)

	// 4. Describe Table Tool
	describeTableToolTool := mcp.NewTool("describeTable",
		mcp.WithDescription("Get column information for a table"),
		mcp.WithString("table",
			mcp.Required(),
			mcp.Description("Table name"),
		),
		mcp.WithString("schema",
			mcp.Description("Database schema name"),
			mcp.DefaultString("public"),
		),
	)

	// 5. Sample Rows Tool
	sampleRowsToolTool := mcp.NewTool("sampleRows",
		mcp.WithDescription("Get sample rows from a table"),
		mcp.WithString("table",
			mcp.Required(),
			mcp.Description("Table name"),
		),
		mcp.WithString("schema",
			mcp.Description("Database schema name"),
			mcp.DefaultString("public"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of rows to return"),
			mcp.DefaultNumber(5),
		),
	)

	// 6. Foreign Keys Tool
	foreignKeysToolTool := mcp.NewTool("getForeignKeys",
		mcp.WithDescription("Get foreign key relationships for a table"),
		mcp.WithString("table",
			mcp.Required(),
			mcp.Description("Table name"),
		),
		mcp.WithString("schema",
			mcp.Description("Database schema name"),
			mcp.DefaultString("public"),
		),
	)

	// 7. List Schemas Tool
	listSchemasToolTool := mcp.NewTool("listSchemas",
		mcp.WithDescription("List all schemas in the database"),
	)

	// Add tool handlers

	// 1. Execute Query Handler
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

	// 2. Full Table Schema Handler
	mcpSrv.AddTool(fullSchemaToolTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		table := request.Params.Arguments["table"].(string)
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}

		// Create a mock HTTP request to reuse the existing handler logic
		url := fmt.Sprintf("/schema/full?schema=%s&table=%s", schema, table)
		req, _ := http.NewRequest("GET", url, nil)
		rw := newResponseRecorder()

		// Call the existing handler
		server.FullTableSchemaHandler(dbConn)(rw, req)

		if rw.statusCode != http.StatusOK {
			return mcp.NewToolResultError(fmt.Sprintf("Error getting table schema: %s", rw.body.String())), nil
		}

		return mcp.NewToolResultText(rw.body.String()), nil
	})

	// 3. List Tables Handler
	mcpSrv.AddTool(listTablesToolTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}

		// Create a mock HTTP request to reuse the existing handler logic
		url := fmt.Sprintf("/schema/tables?schema=%s", schema)
		req, _ := http.NewRequest("GET", url, nil)
		rw := newResponseRecorder()

		// Call the existing handler
		server.ListTablesHandler(dbConn)(rw, req)

		if rw.statusCode != http.StatusOK {
			return mcp.NewToolResultError(fmt.Sprintf("Error listing tables: %s", rw.body.String())), nil
		}

		return mcp.NewToolResultText(rw.body.String()), nil
	})

	// 4. Describe Table Handler
	mcpSrv.AddTool(describeTableToolTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		table := request.Params.Arguments["table"].(string)
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}

		// Create a mock HTTP request to reuse the existing handler logic
		url := fmt.Sprintf("/schema/describe?schema=%s&table=%s", schema, table)
		req, _ := http.NewRequest("GET", url, nil)
		rw := newResponseRecorder()

		// Call the existing handler
		server.DescribeTableHandler(dbConn)(rw, req)

		if rw.statusCode != http.StatusOK {
			return mcp.NewToolResultError(fmt.Sprintf("Error describing table: %s", rw.body.String())), nil
		}

		return mcp.NewToolResultText(rw.body.String()), nil
	})

	// 5. Sample Rows Handler
	mcpSrv.AddTool(sampleRowsToolTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		table := request.Params.Arguments["table"].(string)
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}

		// Create a mock HTTP request to reuse the existing handler logic
		url := fmt.Sprintf("/schema/sample?schema=%s&table=%s", schema, table)
		req, _ := http.NewRequest("GET", url, nil)
		rw := newResponseRecorder()

		// Call the existing handler
		server.SampleRowsHandler(dbConn)(rw, req)

		if rw.statusCode != http.StatusOK {
			return mcp.NewToolResultError(fmt.Sprintf("Error getting sample rows: %s", rw.body.String())), nil
		}

		return mcp.NewToolResultText(rw.body.String()), nil
	})

	// 6. Foreign Keys Handler
	mcpSrv.AddTool(foreignKeysToolTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		table := request.Params.Arguments["table"].(string)
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}

		// Create a mock HTTP request to reuse the existing handler logic
		url := fmt.Sprintf("/schema/foreign_keys?schema=%s&table=%s", schema, table)
		req, _ := http.NewRequest("GET", url, nil)
		rw := newResponseRecorder()

		// Call the existing handler
		server.ForeignKeysHandler(dbConn)(rw, req)

		if rw.statusCode != http.StatusOK {
			return mcp.NewToolResultError(fmt.Sprintf("Error getting foreign keys: %s", rw.body.String())), nil
		}

		return mcp.NewToolResultText(rw.body.String()), nil
	})

	// 7. List Schemas Handler
	mcpSrv.AddTool(listSchemasToolTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Create a mock HTTP request to reuse the existing handler logic
		req, _ := http.NewRequest("GET", "/schema/list_schemas", nil)
		rw := newResponseRecorder()

		// Call the existing handler
		server.ListSchemasHandler(dbConn)(rw, req)

		if rw.statusCode != http.StatusOK {
			return mcp.NewToolResultError(fmt.Sprintf("Error listing schemas: %s", rw.body.String())), nil
		}

		return mcp.NewToolResultText(rw.body.String()), nil
	})

	// Start the HTTP server
	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// responseRecorder is a custom implementation of http.ResponseWriter to capture response data
type responseRecorder struct {
	headers    http.Header
	body       *bytes.Buffer
	statusCode int
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		headers:    make(http.Header),
		body:       new(bytes.Buffer),
		statusCode: http.StatusOK,
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.headers
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
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
