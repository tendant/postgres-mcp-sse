package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/lib/pq"
)

func getSchemaParam(r *http.Request) string {
	schema := r.URL.Query().Get("schema")
	if schema == "" {
		return "public"
	}
	return schema
}

type QueryRequest struct {
	Schema     string        `json:"schema"`
	Query      string        `json:"query"`
	Args       []interface{} `json:"args"`
	Broadcast  bool          `json:"broadcast,omitempty"`
	EventName  string        `json:"event_name,omitempty"`
}

// Event represents a server-sent event
type Event struct {
	Name string
	Data interface{}
}

// NewEvent creates a new event with the given name and data
func NewEvent(name string, data interface{}) Event {
	return Event{
		Name: name,
		Data: data,
	}
}

// HubInterface defines the interface for a Hub that can broadcast events
type HubInterface interface {
	// Broadcast is a channel for sending events
	Broadcast() chan<- Event
}

func ExecuteQueryHandler(db *sql.DB, hub HubInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req QueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid input", http.StatusBadRequest)
			return
		}
		if req.Query == "" {
			http.Error(w, "Missing SQL query", http.StatusBadRequest)
			return
		}
		if req.Schema == "" {
			req.Schema = "public"
		}
		if req.EventName == "" {
			req.EventName = "query_result"
		}

		_, err := db.Exec(fmt.Sprintf("SET search_path TO %s", pq.QuoteIdentifier(req.Schema)))
		if err != nil {
			http.Error(w, "Failed to set schema: "+err.Error(), http.StatusInternalServerError)
			return
		}

		rows, err := db.Query(req.Query, req.Args...)
		if err != nil {
			http.Error(w, "Query error: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer rows.Close()

		cols, _ := rows.Columns()
		var results []map[string]interface{}

		for rows.Next() {
			columnVals := make([]interface{}, len(cols))
			columnPtrs := make([]interface{}, len(cols))
			for i := range columnVals {
				columnPtrs[i] = &columnVals[i]
			}
			rows.Scan(columnPtrs...)
			rowMap := make(map[string]interface{})
			for i, col := range cols {
				rowMap[col] = convertValue(columnVals[i])
			}
			results = append(results, rowMap)
		}

		resp := map[string]interface{}{
			"columns": cols,
			"rows":    results,
		}

		if req.Broadcast {
			hub.Broadcast() <- NewEvent(req.EventName, resp)
		}

		json.NewEncoder(w).Encode(resp)
	}
}

func FullTableSchemaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := getSchemaParam(r)
		table := r.URL.Query().Get("table")
		if table == "" {
			http.Error(w, "Missing table parameter", http.StatusBadRequest)
			return
		}

		type Column struct {
			Name         string `json:"name"`
			Type         string `json:"type"`
			Nullable     string `json:"nullable"`
			DefaultValue string `json:"default,omitempty"`
		}
		type FKConstraint struct {
			ConstraintName string `json:"constraint_name"`
			SourceTable    string `json:"source_table"`
			SourceColumn   string `json:"source_column"`
			TargetTable    string `json:"target_table"`
			TargetColumn   string `json:"target_column"`
		}

		var columns []Column
		var foreignKeys []FKConstraint
		var samples []map[string]interface{}

		colRows, err := db.Query(`
			SELECT column_name, data_type, is_nullable, column_default
			FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2;
		`, schema, table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer colRows.Close()
		for colRows.Next() {
			var col Column
			colRows.Scan(&col.Name, &col.Type, &col.Nullable, &col.DefaultValue)
			columns = append(columns, col)
		}

		query := fmt.Sprintf(`SELECT * FROM %s.%s LIMIT 5`, pq.QuoteIdentifier(schema), pq.QuoteIdentifier(table))
		sampleRows, err := db.Query(query)
		if err == nil {
			defer sampleRows.Close()
			cols, _ := sampleRows.Columns()
			for sampleRows.Next() {
				columnVals := make([]interface{}, len(cols))
				columnPtrs := make([]interface{}, len(cols))
				for i := range columnVals {
					columnPtrs[i] = &columnVals[i]
				}
				sampleRows.Scan(columnPtrs...)
				rowMap := make(map[string]interface{})
				for i, col := range cols {
					rowMap[col] = convertValue(columnVals[i])
				}
				samples = append(samples, rowMap)
			}
		}

		fkRows, err := db.Query(`
			SELECT
				tc.constraint_name, tc.table_name, kcu.column_name,
				ccu.table_name, ccu.column_name
			FROM information_schema.table_constraints AS tc
			JOIN information_schema.key_column_usage AS kcu
			  ON tc.constraint_name = kcu.constraint_name
			  AND tc.table_schema = kcu.table_schema
			JOIN information_schema.constraint_column_usage AS ccu
			  ON ccu.constraint_name = tc.constraint_name
			  AND ccu.table_schema = tc.table_schema
			WHERE tc.constraint_type = 'FOREIGN KEY'
			  AND tc.table_schema = $1
			  AND (tc.table_name = $2 OR ccu.table_name = $2);
		`, schema, table)
		if err == nil {
			defer fkRows.Close()
			for fkRows.Next() {
				var fk FKConstraint
				fkRows.Scan(&fk.ConstraintName, &fk.SourceTable, &fk.SourceColumn, &fk.TargetTable, &fk.TargetColumn)
				foreignKeys = append(foreignKeys, fk)
			}
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"table":        table,
			"columns":      columns,
			"sample_rows":  samples,
			"foreign_keys": foreignKeys,
		})
	}
}

func ListTablesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := getSchemaParam(r)
		rows, err := db.Query(`
			SELECT table_name
			FROM information_schema.tables
			WHERE table_schema = $1
			ORDER BY table_name;
		`, schema)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var tables []string
		for rows.Next() {
			var table string
			rows.Scan(&table)
			tables = append(tables, table)
		}
		json.NewEncoder(w).Encode(tables)
	}
}

func DescribeTableHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := getSchemaParam(r)
		table := r.URL.Query().Get("table")
		if table == "" {
			http.Error(w, "Missing table parameter", http.StatusBadRequest)
			return
		}
		rows, err := db.Query(`
			SELECT column_name, data_type, is_nullable
			FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2;
		`, schema, table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type Column struct {
			Name         string `json:"name"`
			Type         string `json:"type"`
			Nullable     string `json:"nullable"`
			DefaultValue string `json:"default,omitempty"`
		}
		var columns []Column
		for rows.Next() {
			var col Column
			rows.Scan(&col.Name, &col.Type, &col.Nullable)
			columns = append(columns, col)
		}
		json.NewEncoder(w).Encode(columns)
	}
}

func SampleRowsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := getSchemaParam(r)
		table := r.URL.Query().Get("table")
		if table == "" {
			http.Error(w, "Missing table parameter", http.StatusBadRequest)
			return
		}
		query := fmt.Sprintf("SELECT * FROM %s.%s LIMIT 5", pq.QuoteIdentifier(schema), pq.QuoteIdentifier(table))
		rows, err := db.Query(query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		cols, _ := rows.Columns()
		result := []map[string]interface{}{}
		for rows.Next() {
			columnVals := make([]interface{}, len(cols))
			columnPtrs := make([]interface{}, len(cols))
			for i := range columnVals {
				columnPtrs[i] = &columnVals[i]
			}
			rows.Scan(columnPtrs...)
			rowMap := make(map[string]interface{})
			for i, col := range cols {
				rowMap[col] = convertValue(columnVals[i])
			}
			result = append(result, rowMap)
		}
		json.NewEncoder(w).Encode(result)
	}
}

func ForeignKeysHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := getSchemaParam(r)
		table := r.URL.Query().Get("table")
		if table == "" {
			http.Error(w, "Missing table parameter", http.StatusBadRequest)
			return
		}

		rows, err := db.Query(`
			SELECT
				tc.constraint_name, tc.table_name, kcu.column_name,
				ccu.table_name, ccu.column_name
			FROM information_schema.table_constraints AS tc
			JOIN information_schema.key_column_usage AS kcu
			  ON tc.constraint_name = kcu.constraint_name
			  AND tc.table_schema = kcu.table_schema
			JOIN information_schema.constraint_column_usage AS ccu
			  ON ccu.constraint_name = tc.constraint_name
			  AND ccu.table_schema = tc.table_schema
			WHERE tc.constraint_type = 'FOREIGN KEY'
			  AND tc.table_schema = $1
			  AND (tc.table_name = $2 OR ccu.table_name = $2);
		`, schema, table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type FKConstraint struct {
			ConstraintName string `json:"constraint_name"`
			SourceTable    string `json:"source_table"`
			SourceColumn   string `json:"source_column"`
			TargetTable    string `json:"target_table"`
			TargetColumn   string `json:"target_column"`
		}
		var constraints []FKConstraint
		for rows.Next() {
			var fk FKConstraint
			rows.Scan(&fk.ConstraintName, &fk.SourceTable, &fk.SourceColumn, &fk.TargetTable, &fk.TargetColumn)
			constraints = append(constraints, fk)
		}
		json.NewEncoder(w).Encode(constraints)
	}
}

func ListSchemasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT schema_name FROM information_schema.schemata ORDER BY schema_name;
		`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var schemas []string
		for rows.Next() {
			var schema string
			rows.Scan(&schema)
			schemas = append(schemas, schema)
		}
		json.NewEncoder(w).Encode(schemas)
	}
}
