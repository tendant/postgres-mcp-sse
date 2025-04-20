package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/tendant/postgres-mcp-sse/internal/db"
	"github.com/tendant/postgres-mcp-sse/internal/server"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// EventBroadcaster implements the server.HubInterface for compatibility with existing code
// while also forwarding events to SSE clients via the MCP server
type EventBroadcaster struct {
	broadcastCh chan server.Event
	sseServer   *mcpserver.SSEServer
}

// NewEventBroadcaster creates a new EventBroadcaster
func NewEventBroadcaster(sseServer *mcpserver.SSEServer) *EventBroadcaster {
	broadcaster := &EventBroadcaster{
		broadcastCh: make(chan server.Event),
		sseServer:   sseServer,
	}

	// Start a goroutine to handle events
	go broadcaster.processEvents()

	return broadcaster
}

// processEvents handles incoming events and broadcasts them via SSE
func (b *EventBroadcaster) processEvents() {
	for event := range b.broadcastCh {
		// Convert the event data to JSON
		eventData, err := json.Marshal(event.Data)
		if err != nil {
			log.Printf("Error marshaling event data: %v", err)
			continue
		}

		// Create a notification and send it to all clients
		b.sseServer.SendEvent(event.Name, string(eventData))
	}
}

// Broadcast returns the channel for sending events
func (b *EventBroadcaster) Broadcast() chan<- server.Event {
	return b.broadcastCh
}

// registerMCPTools registers all the MCP tools with the MCP server
func registerMCPTools(mcpServer *mcpserver.MCPServer, dbConn *sql.DB, broadcaster *EventBroadcaster) {
	// 1. Execute Query Tool
	executeQueryTool := mcp.NewTool("executeQuery",
		mcp.WithDescription("Execute a SQL query against the database"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("SQL query to execute"),
		),
		mcp.WithString("schema",
			mcp.Description("Database schema to use"),
			mcp.DefaultString("public"),
		),
		mcp.WithBoolean("broadcast",
			mcp.Description("Whether to broadcast the result as an event"),
		),
		mcp.WithString("eventName",
			mcp.Description("Name of the event to broadcast"),
			mcp.DefaultString("query_result"),
		),
	)

	mcpServer.AddTool(executeQueryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := request.Params.Arguments["query"].(string)
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}
		broadcast, _ := request.Params.Arguments["broadcast"].(bool)
		eventName, _ := request.Params.Arguments["eventName"].(string)
		if eventName == "" {
			eventName = "query_result"
		}

		// Execute the query
		result, err := server.ExecuteQuery(dbConn, schema, query, nil)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Query error: %v", err)), nil
		}

		// Broadcast the result if requested
		if broadcast {
			broadcaster.Broadcast() <- server.NewEvent(eventName, result)
		}

		// Convert result to JSON
		resultJSON, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// 2. List Schemas Tool
	listSchemasTool := mcp.NewTool("listSchemas",
		mcp.WithDescription("List all schemas in the database"),
	)

	mcpServer.AddTool(listSchemasTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		schemas, err := server.ListSchemas(dbConn)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error listing schemas: %v", err)), nil
		}

		// Convert result to JSON
		resultJSON, _ := json.Marshal(schemas)
		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// 3. List Tables Tool
	listTablesTool := mcp.NewTool("listTables",
		mcp.WithDescription("List all tables in a schema"),
		mcp.WithString("schema",
			mcp.Description("Database schema name"),
			mcp.DefaultString("public"),
		),
	)

	mcpServer.AddTool(listTablesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}

		tables, err := server.ListTables(dbConn, schema)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error listing tables: %v", err)), nil
		}

		// Convert result to JSON
		resultJSON, _ := json.Marshal(tables)
		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// 4. Get Full Table Schema Tool
	getFullTableSchemaTool := mcp.NewTool("getFullTableSchema",
		mcp.WithDescription("Get full schema information for a table"),
		mcp.WithString("table",
			mcp.Required(),
			mcp.Description("Table name"),
		),
		mcp.WithString("schema",
			mcp.Description("Database schema name"),
			mcp.DefaultString("public"),
		),
	)

	mcpServer.AddTool(getFullTableSchemaTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		table := request.Params.Arguments["table"].(string)
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}

		result, err := server.GetFullTableSchema(dbConn, schema, table)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error getting table schema: %v", err)), nil
		}

		// Convert result to JSON
		resultJSON, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// 5. Describe Table Tool
	describeTableTool := mcp.NewTool("describeTable",
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

	mcpServer.AddTool(describeTableTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		table := request.Params.Arguments["table"].(string)
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}

		columns, err := server.DescribeTable(dbConn, schema, table)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error describing table: %v", err)), nil
		}

		// Convert result to JSON
		resultJSON, _ := json.Marshal(columns)
		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// 6. Sample Rows Tool
	sampleRowsTool := mcp.NewTool("sampleRows",
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

	mcpServer.AddTool(sampleRowsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		table := request.Params.Arguments["table"].(string)
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}
		limit := 5
		if limitVal, ok := request.Params.Arguments["limit"].(float64); ok {
			limit = int(limitVal)
		}

		result, err := server.SampleRows(dbConn, schema, table, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error getting sample rows: %v", err)), nil
		}

		// Convert result to JSON
		resultJSON, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// 7. Get Foreign Keys Tool
	getForeignKeysTool := mcp.NewTool("getForeignKeys",
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

	mcpServer.AddTool(getForeignKeysTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		table := request.Params.Arguments["table"].(string)
		schema, ok := request.Params.Arguments["schema"].(string)
		if !ok {
			schema = "public"
		}

		foreignKeys, err := server.GetForeignKeys(dbConn, schema, table)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error getting foreign keys: %v", err)), nil
		}

		// Convert result to JSON
		resultJSON, _ := json.Marshal(foreignKeys)
		return mcp.NewToolResultText(string(resultJSON)), nil
	})
}

// setupRoutes sets up the HTTP routes for the server
func setupRoutes(mux *http.ServeMux, sseServer *mcpserver.SSEServer, dbConn *sql.DB, broadcaster *EventBroadcaster) {
	// Set up database query handlers (keep for backward compatibility)
	mux.HandleFunc("/query/execute", server.ExecuteQueryHandler(dbConn, broadcaster))
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

	// Create a new MCP server with logging and recovery middleware
	mcpServer := mcpserver.NewMCPServer(
		"Postgres MCP Server",
		"1.0.0",
		mcpserver.WithResourceCapabilities(true, true), // Enable SSE and JSON-RPC
		mcpserver.WithLogging(),
		mcpserver.WithRecovery(),
	)

	// Create an SSE server for event streaming
	sseServer := mcpserver.NewSSEServer(mcpServer,
		mcpserver.WithSSEEndpoint("/events"),
		mcpserver.WithMessageEndpoint("/mcp"),
		mcpserver.WithKeepAlive(true),
		mcpserver.WithKeepAliveInterval(15 * time.Second),
	)

	// Create an event broadcaster that adapts to our existing interface
	broadcaster := NewEventBroadcaster(sseServer)

	// Register all MCP tools
	registerMCPTools(mcpServer, dbConn, broadcaster)

	// Set up HTTP routes
	mux := http.NewServeMux()
	
	// Add MCP and SSE endpoints
	// The SSEServer already registers its handlers with the provided mux
	
	// Add legacy endpoints
	setupRoutes(mux, sseServer, dbConn, broadcaster)

	// Start the HTTP server
	log.Printf("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// executeQuery executes a SQL query and returns the results
func executeQuery(db *sql.DB, schema, query string) (map[string]interface{}, error) {
	// Set the search path to the specified schema
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
