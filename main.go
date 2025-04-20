package main

import (
	"database/sql"
	"log"
	"net/http"

	"your_project/internal/db"
	"your_project/internal/server"

	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	dbConn, err := db.InitPostgres("postgres://user:password@postgres:5432/mydb?sslmode=disable")
	if err != nil {
		log.Fatalf("DB error: %v", err)
	}
	defer dbConn.Close()

	hub := mcp.NewHub()
	go hub.Run()

	http.HandleFunc("/events", hub.ServeHTTP)
	http.HandleFunc("/query/execute", server.ExecuteQueryHandler(dbConn, hub))
	http.HandleFunc("/schema/full", server.FullTableSchemaHandler(dbConn))
	http.HandleFunc("/schema/tables", server.ListTablesHandler(dbConn))
	http.HandleFunc("/schema/describe", server.DescribeTableHandler(dbConn))
	http.HandleFunc("/schema/sample", server.SampleRowsHandler(dbConn))
	http.HandleFunc("/schema/foreign_keys", server.ForeignKeysHandler(dbConn))
	http.HandleFunc("/schema/list_schemas", server.ListSchemasHandler(dbConn))

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
