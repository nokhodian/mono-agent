package dbnodes

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
	"github.com/monoes/monoes-agent/internal/workflow"
)

// PostgresNode executes PostgreSQL queries.
// Type: "db.postgres"
type PostgresNode struct{}

func (n *PostgresNode) Type() string { return "db.postgres" }

func (n *PostgresNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	operation, _ := config["operation"].(string)
	if operation == "" {
		operation = "select"
	}

	host, _ := config["host"].(string)
	if host == "" {
		host = "localhost"
	}
	port := 5432
	if v, ok := config["port"].(float64); ok {
		port = int(v)
	}
	database, _ := config["database"].(string)
	username, _ := config["username"].(string)
	password, _ := config["password"].(string)

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, username, password, database)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("db.postgres: open failed: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("db.postgres: ping failed: %w", err)
	}

	query, _ := config["query"].(string)
	var params []interface{}
	if rawParams, ok := config["params"].([]interface{}); ok {
		params = rawParams
	}

	table, _ := config["table"].(string)
	dataMap, _ := config["data"].(map[string]interface{})

	if query == "" && table != "" {
		var buildErr error
		query, params, buildErr = buildPostgresQuery(operation, table, dataMap, params)
		if buildErr != nil {
			return nil, fmt.Errorf("db.postgres: %w", buildErr)
		}
	}
	if query == "" {
		return nil, fmt.Errorf("db.postgres: 'query' or 'table' is required")
	}

	switch operation {
	case "select":
		rows, err := db.QueryContext(ctx, query, params...)
		if err != nil {
			return nil, fmt.Errorf("db.postgres: query failed: %w", err)
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("db.postgres: columns failed: %w", err)
		}

		var items []workflow.Item
		for rows.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, fmt.Errorf("db.postgres: scan failed: %w", err)
			}
			rowData := make(map[string]interface{}, len(cols))
			for i, col := range cols {
				v := values[i]
				if b, ok := v.([]byte); ok {
					rowData[col] = string(b)
				} else {
					rowData[col] = v
				}
			}
			items = append(items, workflow.NewItem(rowData))
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("db.postgres: rows error: %w", err)
		}
		return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil

	default: // insert, update, delete, execute
		result, err := db.ExecContext(ctx, query, params...)
		if err != nil {
			return nil, fmt.Errorf("db.postgres: exec failed: %w", err)
		}
		rowsAffected, _ := result.RowsAffected()
		// Postgres driver doesn't support LastInsertId — use 0
		item := workflow.NewItem(map[string]interface{}{
			"rows_affected":  rowsAffected,
			"last_insert_id": int64(0),
		})
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{item}}}, nil
	}
}

// buildPostgresQuery constructs a simple SQL query for the given operation.
// Uses $1, $2, ... placeholders for PostgreSQL.
func buildPostgresQuery(operation, table string, data map[string]interface{}, existingParams []interface{}) (string, []interface{}, error) {
	switch operation {
	case "insert":
		if len(data) == 0 {
			return "", nil, fmt.Errorf("insert requires 'data'")
		}
		cols := make([]string, 0, len(data))
		placeholders := make([]string, 0, len(data))
		params := make([]interface{}, 0, len(data))
		idx := 1
		for k, v := range data {
			cols = append(cols, `"`+k+`"`)
			placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
			params = append(params, v)
			idx++
		}
		q := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s)`, table, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
		return q, params, nil

	case "update":
		if len(data) == 0 {
			return "", nil, fmt.Errorf("update requires 'data'")
		}
		sets := make([]string, 0, len(data))
		params := make([]interface{}, 0, len(data)+len(existingParams))
		idx := 1
		for k, v := range data {
			sets = append(sets, fmt.Sprintf(`"%s" = $%d`, k, idx))
			params = append(params, v)
			idx++
		}
		params = append(params, existingParams...)
		q := fmt.Sprintf(`UPDATE "%s" SET %s`, table, strings.Join(sets, ", "))
		if len(existingParams) > 0 {
			q += fmt.Sprintf(" WHERE $%d", idx)
		}
		return q, params, nil

	case "delete":
		q := fmt.Sprintf(`DELETE FROM "%s"`, table)
		return q, existingParams, nil

	case "select":
		q := fmt.Sprintf(`SELECT * FROM "%s"`, table)
		return q, existingParams, nil

	default:
		return "", nil, fmt.Errorf("cannot auto-build query for operation %q", operation)
	}
}
