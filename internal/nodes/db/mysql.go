package dbnodes

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/monoes/monoes-agent/internal/workflow"
)

var validMySQLIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)

func validateMySQLIdentifier(name string) error {
	if !validMySQLIdentifier.MatchString(name) {
		return fmt.Errorf("invalid SQL identifier: %q", name)
	}
	return nil
}

// MySQLNode executes MySQL queries.
// Type: "db.mysql"
type MySQLNode struct{}

func (n *MySQLNode) Type() string { return "db.mysql" }

func (n *MySQLNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	operation, _ := config["operation"].(string)
	if operation == "" {
		operation = "select"
	}

	host, _ := config["host"].(string)
	if host == "" {
		host = "localhost"
	}
	port := 3306
	if v, ok := config["port"].(float64); ok {
		port = int(v)
	}
	database, _ := config["database"].(string)
	username, _ := config["username"].(string)
	password, _ := config["password"].(string)

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", username, password, host, port, database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("db.mysql: open failed: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("db.mysql: ping failed: %w", err)
	}

	query, _ := config["query"].(string)
	var params []interface{}
	if rawParams, ok := config["params"].([]interface{}); ok {
		params = rawParams
	}

	// Build query from table+data if no raw query
	table, _ := config["table"].(string)
	dataMap, _ := config["data"].(map[string]interface{})

	if query == "" && table != "" {
		var buildErr error
		query, params, buildErr = buildMySQLQuery(operation, table, dataMap, params, config)
		if buildErr != nil {
			return nil, fmt.Errorf("db.mysql: %w", buildErr)
		}
	}
	if query == "" {
		return nil, fmt.Errorf("db.mysql: 'query' or 'table' is required")
	}

	switch operation {
	case "select":
		rows, err := db.QueryContext(ctx, query, params...)
		if err != nil {
			return nil, fmt.Errorf("db.mysql: query failed: %w", err)
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("db.mysql: columns failed: %w", err)
		}

		var items []workflow.Item
		for rows.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, fmt.Errorf("db.mysql: scan failed: %w", err)
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
			return nil, fmt.Errorf("db.mysql: rows error: %w", err)
		}
		return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil

	default: // insert, update, delete, execute
		result, err := db.ExecContext(ctx, query, params...)
		if err != nil {
			return nil, fmt.Errorf("db.mysql: exec failed: %w", err)
		}
		rowsAffected, _ := result.RowsAffected()
		lastInsertID, _ := result.LastInsertId()
		item := workflow.NewItem(map[string]interface{}{
			"rows_affected":  rowsAffected,
			"last_insert_id": lastInsertID,
		})
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{item}}}, nil
	}
}

func buildMySQLQuery(operation, table string, data map[string]interface{}, existingParams []interface{}, config map[string]interface{}) (string, []interface{}, error) {
	if err := validateMySQLIdentifier(table); err != nil {
		return "", nil, fmt.Errorf("invalid table name: %w", err)
	}
	for k := range data {
		if err := validateMySQLIdentifier(k); err != nil {
			return "", nil, fmt.Errorf("invalid column name: %w", err)
		}
	}

	whereClause, _ := config["where"].(string)

	switch operation {
	case "insert":
		if len(data) == 0 {
			return "", nil, fmt.Errorf("insert requires 'data'")
		}
		cols := make([]string, 0, len(data))
		placeholders := make([]string, 0, len(data))
		params := make([]interface{}, 0, len(data))
		for k, v := range data {
			cols = append(cols, "`"+k+"`")
			placeholders = append(placeholders, "?")
			params = append(params, v)
		}
		q := fmt.Sprintf("INSERT INTO `%s` (%s) VALUES (%s)", table, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
		return q, params, nil

	case "update":
		if len(data) == 0 {
			return "", nil, fmt.Errorf("update requires 'data'")
		}
		if whereClause == "" && len(existingParams) == 0 {
			return "", nil, fmt.Errorf("UPDATE requires a WHERE clause")
		}
		sets := make([]string, 0, len(data))
		params := make([]interface{}, 0, len(data)+len(existingParams))
		for k, v := range data {
			sets = append(sets, "`"+k+"` = ?")
			params = append(params, v)
		}
		params = append(params, existingParams...)
		q := fmt.Sprintf("UPDATE `%s` SET %s", table, strings.Join(sets, ", "))
		if whereClause != "" {
			q += " WHERE " + whereClause
		}
		return q, params, nil

	case "delete":
		if whereClause == "" {
			return "", nil, fmt.Errorf("DELETE requires a WHERE clause")
		}
		q := fmt.Sprintf("DELETE FROM `%s` WHERE %s", table, whereClause)
		return q, existingParams, nil

	case "select":
		q := fmt.Sprintf("SELECT * FROM `%s`", table)
		return q, existingParams, nil

	default:
		return "", nil, fmt.Errorf("cannot auto-build query for operation %q", operation)
	}
}
