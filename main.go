package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/tendant/postgres-mcp-sse/internal/db"
	"github.com/tendant/postgres-mcp-sse/internal/server"
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

// createMCPHandler creates an HTTP handler for the MCP JSON-RPC endpoint
func createMCPHandler(dbConn *sql.DB, hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			result = handleInitialize()
		case "tools/list":
			result = handleToolsList()
		case "tools/call":
			// Handle tool calls
			toolName, _ := params["name"].(string)
			toolArgs, _ := params["arguments"].(map[string]interface{})
			result = handleToolCall(dbConn, hub, toolName, toolArgs)
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
	}
}

// handleInitialize handles the initialize method for the MCP protocol
func handleInitialize() interface{} {
	return map[string]interface{}{
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
}

// handleToolsList returns the list of available tools
func handleToolsList() interface{} {
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
	return map[string]interface{}{
		"tools": tools,
	}
}

// handleToolCall processes a tool call based on the tool name and arguments
func handleToolCall(dbConn *sql.DB, hub *Hub, toolName string, toolArgs map[string]interface{}) interface{} {
	switch toolName {
	case "executeQuery":
		return handleExecuteQuery(dbConn, hub, toolArgs)
	case "listSchemas":
		return handleListSchemas(dbConn)
	case "listTables":
		return handleListTables(dbConn, toolArgs)
	case "getFullTableSchema":
		return handleGetFullTableSchema(dbConn, toolArgs)
	case "describeTable":
		return handleDescribeTable(dbConn, toolArgs)
	case "sampleRows":
		return handleSampleRows(dbConn, toolArgs)
	case "getForeignKeys":
		return handleGetForeignKeys(dbConn, toolArgs)
	default:
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Tool '%s' not implemented yet", toolName),
				},
			},
		}
	}
}

// handleExecuteQuery handles the executeQuery tool call
func handleExecuteQuery(dbConn *sql.DB, hub *Hub, toolArgs map[string]interface{}) interface{} {
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

	// Extract any arguments if provided
	var args []interface{}
	if argsVal, ok := toolArgs["args"].([]interface{}); ok {
		args = argsVal
	}

	// Execute the query directly using the core function
	result, err := server.ExecuteQuery(dbConn, schema, query, args)
	if err != nil {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Error: %s", err.Error()),
				},
			},
		}
	}

	// If broadcast is requested, send the event
	if broadcast {
		hub.Broadcast() <- server.NewEvent(eventName, result)
	}

	// Convert result to JSON for response
	resultJSON, _ := json.Marshal(result)
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultJSON),
			},
		},
	}
}

// handleListSchemas handles the listSchemas tool call
func handleListSchemas(dbConn *sql.DB) interface{} {
	// Call the core function directly
	schemas, err := server.ListSchemas(dbConn)
	if err != nil {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Error: %s", err.Error()),
				},
			},
		}
	}

	// Convert result to JSON for response
	resultJSON, _ := json.Marshal(schemas)
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultJSON),
			},
		},
	}
}

// handleListTables handles the listTables tool call
func handleListTables(dbConn *sql.DB, toolArgs map[string]interface{}) interface{} {
	schema, ok := toolArgs["schema"].(string)
	if !ok {
		schema = "public"
	}

	// Call the core function directly
	tables, err := server.ListTables(dbConn, schema)
	if err != nil {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Error: %s", err.Error()),
				},
			},
		}
	}

	// Convert result to JSON for response
	resultJSON, _ := json.Marshal(tables)
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultJSON),
			},
		},
	}
}

// handleGetFullTableSchema handles the getFullTableSchema tool call
func handleGetFullTableSchema(dbConn *sql.DB, toolArgs map[string]interface{}) interface{} {
	table, ok := toolArgs["table"].(string)
	if !ok || table == "" {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "Error: Missing table name",
				},
			},
		}
	}

	schema, ok := toolArgs["schema"].(string)
	if !ok {
		schema = "public"
	}

	// Call the core function directly
	result, err := server.GetFullTableSchema(dbConn, schema, table)
	if err != nil {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Error: %s", err.Error()),
				},
			},
		}
	}

	// Convert result to JSON for response
	resultJSON, _ := json.Marshal(result)
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultJSON),
			},
		},
	}
}

// handleDescribeTable handles the describeTable tool call
func handleDescribeTable(dbConn *sql.DB, toolArgs map[string]interface{}) interface{} {
	table, ok := toolArgs["table"].(string)
	if !ok || table == "" {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "Error: Missing table name",
				},
			},
		}
	}

	schema, ok := toolArgs["schema"].(string)
	if !ok {
		schema = "public"
	}

	// Call the core function directly
	columns, err := server.DescribeTable(dbConn, schema, table)
	if err != nil {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Error: %s", err.Error()),
				},
			},
		}
	}

	// Convert result to JSON for response
	resultJSON, _ := json.Marshal(columns)
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultJSON),
			},
		},
	}
}

// handleSampleRows handles the sampleRows tool call
func handleSampleRows(dbConn *sql.DB, toolArgs map[string]interface{}) interface{} {
	table, ok := toolArgs["table"].(string)
	if !ok || table == "" {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "Error: Missing table name",
				},
			},
		}
	}

	schema, ok := toolArgs["schema"].(string)
	if !ok {
		schema = "public"
	}

	// Get limit if provided
	limit := 5 // Default limit
	if limitVal, ok := toolArgs["limit"].(float64); ok {
		limit = int(limitVal)
	}

	// Call the core function directly
	result, err := server.SampleRows(dbConn, schema, table, limit)
	if err != nil {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Error: %s", err.Error()),
				},
			},
		}
	}

	// Convert result to JSON for response
	resultJSON, _ := json.Marshal(result)
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultJSON),
			},
		},
	}
}

// handleGetForeignKeys handles the getForeignKeys tool call
func handleGetForeignKeys(dbConn *sql.DB, toolArgs map[string]interface{}) interface{} {
	table, ok := toolArgs["table"].(string)
	if !ok || table == "" {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "Error: Missing table name",
				},
			},
		}
	}

	schema, ok := toolArgs["schema"].(string)
	if !ok {
		schema = "public"
	}

	// Call the core function directly
	foreignKeys, err := server.GetForeignKeys(dbConn, schema, table)
	if err != nil {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Error: %s", err.Error()),
				},
			},
		}
	}

	// Convert result to JSON for response
	resultJSON, _ := json.Marshal(foreignKeys)
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(resultJSON),
			},
		},
	}
}

// setupRoutes sets up the HTTP routes for the server
func setupRoutes(mux *http.ServeMux, dbConn *sql.DB, hub *Hub) {
	// Set up SSE events endpoint for backward compatibility
	mux.HandleFunc("/events", hub.ServeHTTP)

	// Set up MCP server HTTP handler
	mux.Handle("/mcp", createMCPHandler(dbConn, hub))

	// Set up database query handlers (keep for backward compatibility)
	mux.HandleFunc("/query/execute", server.ExecuteQueryHandler(dbConn, hub))
	mux.HandleFunc("/schema/full", server.FullTableSchemaHandler(dbConn))
	mux.HandleFunc("/schema/tables", server.ListTablesHandler(dbConn))
	mux.HandleFunc("/schema/describe", server.DescribeTableHandler(dbConn))
	mux.HandleFunc("/schema/sample", server.SampleRowsHandler(dbConn))
	mux.HandleFunc("/schema/foreign_keys", server.ForeignKeysHandler(dbConn))
	mux.HandleFunc("/schema/list_schemas", server.ListSchemasHandler(dbConn))
}

func main() {
	// Initialize Postgres connection
	dbConn, err := db.InitPostgres("postgres://postgres:pwd@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		log.Fatalf("DB error: %v", err)
	}
	defer dbConn.Close()

	// Create a Hub for SSE events
	hub := NewHub()
	go hub.Run()

	// Set up HTTP routes
	mux := http.NewServeMux()
	setupRoutes(mux, dbConn, hub)

	// Start the HTTP server
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
