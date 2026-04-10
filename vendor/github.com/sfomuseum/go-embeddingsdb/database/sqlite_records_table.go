//go:build sqlite

package database

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"text/template"

	sfom_sql "github.com/sfomuseum/go-database/sql"
)

type SQLiteRecordsTable struct {
	sfom_sql.Table
	name string
}

type SQLiteRecordsTableSchemaVars struct {
	TableName string
}

func NewSQLiteRecordsTable(ctx context.Context, uri string) (sfom_sql.Table, error) {

	t := &SQLiteRecordsTable{
		name: "records",
	}

	return t, nil
}

func (t *SQLiteRecordsTable) Name() string {
	return t.name
}

func (t *SQLiteRecordsTable) Schema(*sql.DB) (string, error) {

	tp, err := template.ParseFS(sqlite_schema_fs, "sqlite_*_schema.txt")

	if err != nil {
		return "", err
	}

	tp = tp.Lookup("sqlite_records")

	if tp == nil {
		return "", fmt.Errorf("Missing schema template")
	}

	vars := SQLiteRecordsTableSchemaVars{
		TableName: t.Name(),
	}

	var buf bytes.Buffer
	wr := bufio.NewWriter(&buf)

	err = tp.Execute(wr, vars)

	if err != nil {
		return "", err
	}

	wr.Flush()
	return buf.String(), nil
}

func (t *SQLiteRecordsTable) InitializeTable(context.Context, *sql.DB) error {
	return nil
}

func (t *SQLiteRecordsTable) IndexRecord(context.Context, *sql.DB, *sql.Tx, any) error {
	return fmt.Errorf("Not implemented")
}
