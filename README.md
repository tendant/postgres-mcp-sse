# PostgreSQL MCP SSE Server

A PostgreSQL Model Context Protocol (MCP) Server with Server-Sent Events (SSE) capabilities. This project provides a bridge between PostgreSQL databases and AI assistants through the Model Context Protocol, allowing for real-time database interactions.

## Overview

This server enables AI assistants to interact with PostgreSQL databases through a standardized interface. It provides tools for querying databases, inspecting schema information, and receiving real-time updates through Server-Sent Events (SSE).

Key components:
- PostgreSQL database connection
- MCP server implementation with tools for database operations
- SSE for real-time event broadcasting
- HTTP API endpoints for backward compatibility

## Features

- **PostgreSQL Integration**: Connect to any PostgreSQL database
- **MCP Server**: Implements the Model Context Protocol for AI assistant integration
- **Server-Sent Events (SSE)**: Real-time updates and notifications
- **Database Introspection**: Explore database schemas, tables, and relationships
- **Query Execution**: Run SQL queries with parameter support
- **Docker Support**: Easy deployment with Docker and Docker Compose

## Installation

### Prerequisites

- Docker and Docker Compose (for containerized setup)
- Go 1.24+ (for local development)
- PostgreSQL (if not using Docker)

### Using Docker Compose

1. Clone the repository:
   ```bash
   git clone https://github.com/tendant/postgres-mcp-sse.git
   cd postgres-mcp-sse
   ```

2. Start the services:
   ```bash
   docker-compose up -d
   ```

   This will start both the PostgreSQL database and the MCP server.

### Manual Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/tendant/postgres-mcp-sse.git
   cd postgres-mcp-sse
   ```

2. Set up a PostgreSQL database and initialize it with the schema:
   ```bash
   psql -U postgres -f schema.sql
   psql -U postgres -f seed.sql
   ```

3. Build and run the server:
   ```bash
   go mod tidy
   go build -o mcp-server main.go
   DB_DSN="postgres://postgres:pwd@localhost:5432/postgres?sslmode=disable" ./mcp-server
   ```

## Usage

### Starting the Server

When using Docker Compose:
```bash
docker-compose up -d
```

When running locally:
```bash
DB_DSN="postgres://postgres:pwd@localhost:5432/postgres?sslmode=disable" ./mcp-server
```

The server will start on port 8080 by default. You can customize the port using the `PORT` environment variable.

### HTTP API Examples

Get a list of tables:
```bash
curl http://localhost:8080/schema/tables
```

Describe a table:
```bash
curl "http://localhost:8080/schema/describe?table=users"
```

Get full schema information:
```bash
curl "http://localhost:8080/schema/full?table=orders"
```

Execute a query:
```bash
curl -X POST http://localhost:8080/query/execute \
     -H "Content-Type: application/json" \
     -d '{"query":"SELECT * FROM users LIMIT 1", "schema":"public"}'
```

### MCP Client Example (Go)

```go
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
	err := mcpClient.Initialize(context.Background())
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

	// Test executeQuery tool
	args := map[string]interface{}{
		"query":  "SELECT * FROM users LIMIT 1",
		"schema": "public",
	}
	result, err := mcpClient.CallTool(context.Background(), "executeQuery", args)
	if err != nil {
		log.Fatalf("Failed to call executeQuery tool: %v", err)
	}
	fmt.Printf("Result: %s\n", result.Content[0].Text)
}
```

### Testing with Bash Script

The repository includes a test script that demonstrates both JSON-RPC and SSE functionality:

```bash
./test-mcp.sh
```

## API Documentation

### HTTP Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/query/execute` | POST | Execute a SQL query |
| `/schema/full` | GET | Get full schema information for a table |
| `/schema/tables` | GET | List all tables in a schema |
| `/schema/describe` | GET | Get column information for a table |
| `/schema/sample` | GET | Get sample rows from a table |
| `/schema/foreign_keys` | GET | Get foreign key relationships for a table |
| `/schema/list_schemas` | GET | List all schemas in the database |

### MCP Tools

| Tool Name | Description |
|-----------|-------------|
| `sendNotification` | Send a notification to the client |
| `executeQuery` | Execute a SQL query against the database |
| `listSchemas` | List all schemas in the database |
| `listTables` | List all tables in a schema |
| `getFullTableSchema` | Get full schema information for a table |
| `describeTable` | Get column information for a table |
| `sampleRows` | Get sample rows from a table |
| `getForeignKeys` | Get foreign key relationships for a table |

### SSE Events

The server supports Server-Sent Events (SSE) for real-time updates. Connect to the `/events` endpoint to receive events:

```bash
curl -N "http://localhost:8080/events"
```

Events are sent in the following format:
```
event: [event_name]
data: [event_data_json]
```

## Database Schema

The project includes a simple example schema with two tables:

### Users Table
```sql
CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY,
  email TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT now()
);
```

### Orders Table
```sql
CREATE TABLE IF NOT EXISTS orders (
  id UUID PRIMARY KEY,
  user_id UUID REFERENCES users(id),
  amount NUMERIC NOT NULL,
  created_at TIMESTAMP DEFAULT now()
);
```

To customize the schema, modify the `schema.sql` and `seed.sql` files before starting the containers.

## Development

### Building from Source

```bash
go mod tidy
go build -o mcp-server main.go
```

### Docker Build

To build and push Docker images for multiple platforms:

```bash
make docker-build
```

### Testing

Run the test script to verify functionality:

```bash
./test-mcp.sh
```

Or use the test client:

```bash
go run test-client.go
```

## License

[Add your license information here]

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
