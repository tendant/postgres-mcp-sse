#!/bin/bash

# Initialize the MCP server
echo "Initializing MCP server..."
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocol_version": "2024-11-05",
      "client_name": "curl-test-client",
      "client_version": "1.0.0",
      "capabilities": {}
    }
  }'

echo -e "\n\nListing available tools..."
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list",
    "params": {}
  }'

echo -e "\n\nTesting listSchemas tool..."
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "listSchemas",
      "arguments": {}
    }
  }'

echo -e "\n\nTesting listTables tool..."
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "listTables",
      "arguments": {
        "schema": "public"
      }
    }
  }'

echo -e "\n\nTesting executeQuery tool..."
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 5,
    "method": "tools/call",
    "params": {
      "name": "executeQuery",
      "arguments": {
        "query": "SELECT current_database(), current_schema()",
        "schema": "public"
      }
    }
  }'
