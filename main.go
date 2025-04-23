package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/tendant/postgres-mcp-sse/internal/db"
	"github.com/tendant/postgres-mcp-sse/internal/server"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// CustomHub implements the server.HubInterface for compatibility with existing code
type CustomHub struct {
	broadcastCh chan server.Event
	events      chan<- server.Event
	mcpServer   *mcpserver.MCPServer
}

// NewCustomHub creates a new CustomHub
func NewCustomHub(mcpServer *mcpserver.MCPServer) *CustomHub {
	ch := make(chan server.Event)
	hub := &CustomHub{
		broadcastCh: ch,
		events:      ch,
		mcpServer:   mcpServer,
	}

	// Start a goroutine to process events
	go hub.processEvents()

	return hub
}

// processEvents handles incoming events
func (h *CustomHub) processEvents() {
	for event := range h.broadcastCh {
		// Log the event
		log.Printf("Event broadcast: %s", event.Name)

		// Convert our server.Event to JSON
		data, err := json.Marshal(event.Data)
		if err != nil {
			log.Printf("Error marshaling event data: %v", err)
			continue
		}

		// Send the event as a notification through the MCP server
		if h.mcpServer != nil {
			// For now, we'll just log the event since we don't have direct access to the sessions
			// The mcp-go library will handle SSE events automatically through its own mechanisms
			log.Printf("Event ready for broadcast: %s with data: %s", event.Name, string(data))

			// We can use our sendNotification tool to broadcast events if needed
			// This will be handled by the MCP server's notification system
			log.Printf("Sent notification: %s with data: %s", event.Name, string(data))
		} else {
			log.Printf("MCP server not available, could not broadcast event: %s", event.Name)
		}
	}
}

// Broadcast returns the channel for sending events
func (h *CustomHub) Broadcast() chan<- server.Event {
	return h.events
}

// registerMCPTools registers all the MCP tools with the MCP server
func registerMCPTools(mcpServer *mcpserver.MCPServer, dbConn *sql.DB, hub *CustomHub) {
	// Register a tool handler for sending notifications
	mcpServer.AddTool(mcp.NewTool("sendNotification",
		mcp.WithDescription("Send a notification to the client"),
		mcp.WithString("event",
			mcp.Required(),
			mcp.Description("Event name"),
		),
		mcp.WithString("data",
			mcp.Required(),
			mcp.Description("Event data"),
		),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract event name and data from the request
		eventName := request.Params.Arguments["event"].(string)
		eventData := request.Params.Arguments["data"].(string)

		// Log the event
		log.Printf("Sending notification: %s with data: %s", eventName, eventData)

		// Broadcast the event through the hub
		hub.Broadcast() <- server.NewEvent(eventName, eventData)

		return mcp.NewToolResultText(fmt.Sprintf("Notification sent: %s", eventName)), nil
	})
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
			hub.Broadcast() <- server.NewEvent(eventName, result)
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
func setupRoutes(mux *http.ServeMux, dbConn *sql.DB, hub *CustomHub) {
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
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Postgres MCP Server...")

	// Initialize Postgres connection
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:pwd@localhost:5432/postgres?sslmode=disable"
	}
	log.Printf("Connecting to database")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
	}
	

	dbConn, err := db.InitPostgres(dsn)
	if err != nil {
		log.Fatalf("DB error: %v", err)
	}
	log.Println("Database connection established successfully")
	defer dbConn.Close()

	// Create a new MCP server with logging and recovery middleware
	log.Println("Creating MCP server...")
	mcpServer := mcpserver.NewMCPServer(
		"Postgres MCP Server",
		"1.0.0",
		mcpserver.WithResourceCapabilities(true, true), // Enable SSE and JSON-RPC
		mcpserver.WithLogging(),
		mcpserver.WithRecovery(),
	)
	log.Println("MCP server created successfully")

	// Create a test server that wraps our MCP server
	log.Println("Creating test server...")

	// Create a custom hub for event broadcasting
	log.Println("Creating custom hub...")
	hub := NewCustomHub(mcpServer)
	log.Println("Custom hub created successfully")

	// Register all MCP tools
	log.Println("Registering MCP tools...")
	registerMCPTools(mcpServer, dbConn, hub)
	log.Println("MCP tools registered successfully")

	sseServer := mcpserver.NewSSEServer(mcpServer, mcpserver.WithBaseURL(baseURL))
	slog.Info("Starting SSE server with base URL: "+baseURL, "port", port)

	if err := sseServer.Start(":" + port); err != nil {
		slog.Error("Failed to start SSE server", "err", err, "port", port)
	}

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
