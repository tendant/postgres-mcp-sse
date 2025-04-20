### Curl Examples

# Get table list
curl http://localhost:8080/schema/tables

# Describe a table
curl "http://localhost:8080/schema/describe?table=users"

# Full schema
curl "http://localhost:8080/schema/full?table=orders"

# Execute query
curl -X POST http://localhost:8080/query/execute \
     -H "Content-Type: application/json" \
     -d '{"query":"SELECT * FROM users LIMIT 1", "schema":"public"}'
