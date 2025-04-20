package server

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

// ExecuteQuery executes a SQL query and returns the results
func ExecuteQuery(db *sql.DB, schema, query string, args []interface{}) (map[string]interface{}, error) {
	// Set the schema
	_, err := db.Exec(fmt.Sprintf("SET search_path TO %s", pq.QuoteIdentifier(schema)))
	if err != nil {
		return nil, fmt.Errorf("failed to set schema: %w", err)
	}

	// Execute the query
	rows, err := db.Query(query, args...)
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
		rows.Scan(columnPtrs...)
		rowMap := make(map[string]interface{})
		for i, col := range cols {
			rowMap[col] = columnVals[i]
		}
		results = append(results, rowMap)
	}

	return map[string]interface{}{
		"columns": cols,
		"rows":    results,
	}, nil
}

// ListTables returns a list of tables in the specified schema
func ListTables(db *sql.DB, schema string) ([]string, error) {
	rows, err := db.Query(`
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = $1
		ORDER BY table_name;
	`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		rows.Scan(&table)
		tables = append(tables, table)
	}
	return tables, nil
}

// ListSchemas returns a list of all schemas in the database
func ListSchemas(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT schema_name FROM information_schema.schemata ORDER BY schema_name;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var schema string
		rows.Scan(&schema)
		schemas = append(schemas, schema)
	}
	return schemas, nil
}

// GetFullTableSchema returns detailed schema information for a table
func GetFullTableSchema(db *sql.DB, schema, table string) (map[string]interface{}, error) {
	// Get column information
	rows, err := db.Query(`
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position;
	`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []map[string]interface{}
	for rows.Next() {
		var colName, dataType, isNullable, colDefault sql.NullString
		rows.Scan(&colName, &dataType, &isNullable, &colDefault)
		
		column := map[string]interface{}{
			"name": colName.String,
			"type": dataType.String,
			"nullable": isNullable.String == "YES",
		}
		if colDefault.Valid {
			column["default"] = colDefault.String
		}
		columns = append(columns, column)
	}

	return map[string]interface{}{
		"schema": schema,
		"table":  table,
		"columns": columns,
	}, nil
}

// DescribeTable returns column information for a table
func DescribeTable(db *sql.DB, schema, table string) ([]map[string]interface{}, error) {
	rows, err := db.Query(`
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position;
	`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []map[string]interface{}
	for rows.Next() {
		var colName, dataType, isNullable, colDefault sql.NullString
		rows.Scan(&colName, &dataType, &isNullable, &colDefault)
		
		column := map[string]interface{}{
			"name": colName.String,
			"type": dataType.String,
			"nullable": isNullable.String == "YES",
		}
		if colDefault.Valid {
			column["default"] = colDefault.String
		}
		columns = append(columns, column)
	}

	return columns, nil
}

// SampleRows returns sample rows from a table
func SampleRows(db *sql.DB, schema, table string, limit int) (map[string]interface{}, error) {
	if limit <= 0 {
		limit = 5 // Default limit
	}

	// Set the schema
	_, err := db.Exec(fmt.Sprintf("SET search_path TO %s", pq.QuoteIdentifier(schema)))
	if err != nil {
		return nil, fmt.Errorf("failed to set schema: %w", err)
	}

	// Get sample rows
	query := fmt.Sprintf("SELECT * FROM %s LIMIT %d", pq.QuoteIdentifier(table), limit)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Get column names
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Process results
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
			rowMap[col] = columnVals[i]
		}
		results = append(results, rowMap)
	}

	return map[string]interface{}{
		"columns": cols,
		"rows":    results,
	}, nil
}

// GetForeignKeys returns foreign key relationships for a table
func GetForeignKeys(db *sql.DB, schema, table string) ([]map[string]interface{}, error) {
	rows, err := db.Query(`
		SELECT
			kcu.column_name,
			ccu.table_schema AS foreign_table_schema,
			ccu.table_name AS foreign_table_name,
			ccu.column_name AS foreign_column_name
		FROM
			information_schema.table_constraints AS tc
			JOIN information_schema.key_column_usage AS kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
			JOIN information_schema.constraint_column_usage AS ccu
			ON ccu.constraint_name = tc.constraint_name
			AND ccu.table_schema = tc.table_schema
		WHERE
			tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_schema = $1
			AND tc.table_name = $2;
	`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var foreignKeys []map[string]interface{}
	for rows.Next() {
		var column, foreignSchema, foreignTable, foreignColumn string
		rows.Scan(&column, &foreignSchema, &foreignTable, &foreignColumn)
		
		foreignKeys = append(foreignKeys, map[string]interface{}{
			"column": column,
			"references": map[string]string{
				"schema": foreignSchema,
				"table":  foreignTable,
				"column": foreignColumn,
			},
		})
	}

	return foreignKeys, nil
}
