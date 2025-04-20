#!/bin/bash

# Test both JSON-RPC and SSE functionality

# Start by opening an SSE connection in the background
echo "Opening SSE connection (in background)..."
curl -N "http://localhost:8080/events" > /tmp/sse_output.txt 2>&1 &
SSE_PID=$!

# Give it a moment to establish the connection
sleep 2

# Extract the session ID from the SSE output
if grep -q "sessionId=" /tmp/sse_output.txt; then
  SESSION_ID=$(grep -o "sessionId=[^\"&]*" /tmp/sse_output.txt | head -1 | cut -d= -f2)
  echo "Using server-provided session ID: $SESSION_ID"
else
  echo "Error: Could not extract session ID from SSE output"
  cat /tmp/sse_output.txt
  exit 1
fi

# Initialize the MCP server
echo "Initializing MCP server..."
INIT_RESPONSE=$(curl -s -X POST "http://localhost:8080/mcp?sessionId=$SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"id\": 1,
    \"method\": \"initialize\",
    \"params\": {
      \"protocol_version\": \"2024-11-05\",
      \"client_name\": \"curl-test-client\",
      \"client_version\": \"1.0.0\",
      \"capabilities\": {}
    }
  }")

echo "$INIT_RESPONSE"

# List available tools
echo -e "\nListing available tools..."
TOOLS_RESPONSE=$(curl -s -X POST "http://localhost:8080/mcp?sessionId=$SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"id\": 2,
    \"method\": \"tools/list\",
    \"params\": {}
  }")

echo "$TOOLS_RESPONSE"

# Test listSchemas tool
echo -e "\nTesting listSchemas tool..."
SCHEMAS_RESPONSE=$(curl -s -X POST "http://localhost:8080/mcp?sessionId=$SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"id\": 3,
    \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"listSchemas\",
      \"arguments\": {}
    }
  }")

echo "$SCHEMAS_RESPONSE"

# Test listTables tool
echo -e "\nTesting listTables tool..."
TABLES_RESPONSE=$(curl -s -X POST "http://localhost:8080/mcp?sessionId=$SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"id\": 4,
    \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"listTables\",
      \"arguments\": {
        \"schema\": \"public\"
      }
    }
  }")

echo "$TABLES_RESPONSE"

# Test executeQuery tool with broadcast to test SSE
echo -e "\nTesting executeQuery tool with broadcast..."
QUERY_RESPONSE=$(curl -s -X POST "http://localhost:8080/mcp?sessionId=$SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"id\": 5,
    \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"executeQuery\",
      \"arguments\": {
        \"query\": \"SELECT current_database(), current_user\",
        \"schema\": \"public\",
        \"broadcast\": true,
        \"eventName\": \"test_query_result\"
      }
    }
  }")

echo "$QUERY_RESPONSE"

# Give SSE a moment to receive the broadcast
sleep 2

# Check if we received any SSE events
echo -e "\nChecking SSE events..."
if grep -q "data:" /tmp/sse_output.txt; then
  echo "SSE events received successfully!"
  echo "Last few SSE events:"
  tail -5 /tmp/sse_output.txt
else
  echo "No SSE events received."
fi

# Clean up
echo -e "\nCleaning up..."
kill $SSE_PID 2>/dev/null
rm -f /tmp/sse_output.txt

echo -e "\nTest complete!"
