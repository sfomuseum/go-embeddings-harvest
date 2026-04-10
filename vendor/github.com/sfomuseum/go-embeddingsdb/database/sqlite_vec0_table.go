//go:build sqlite

package database

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"text/template"

	sfom_sql "github.com/sfomuseum/go-database/sql"
)

type SQLiteVec0Table struct {
	sfom_sql.Table
	dimensions   int
	max_distance float32
	max_results  int32
	name         string
	compression  string
}

type SQLiteVec0TableSchemaVars struct {
	Dimensions int
	TableName  string
}

func NewSQLiteVec0Table(ctx context.Context, uri string) (sfom_sql.Table, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	q := u.Query()

	dimensions := 512
	max_distance := float32(1.0)
	max_results := int32(10)
	compression := sqlite_vec_default_compression

	if q.Has("dimensions") {

		v, err := strconv.Atoi(q.Get("dimensions"))

		if err != nil {
			return nil, fmt.Errorf("Invalid ?dimensions= parameter, %w", err)
		}

		dimensions = v
		slog.Debug("Reassign dimensions", "value", dimensions)
	}

	if q.Has("max-distance") {

		v, err := strconv.ParseFloat(q.Get("max-distance"), 64)

		if err != nil {
			return nil, fmt.Errorf("Invalid ?max-distance= parameter, %w", err)
		}

		max_distance = float32(v)
		slog.Debug("Reassign max distance", "value", max_distance)
	}

	if q.Has("max-results") {

		v, err := strconv.Atoi(q.Get("max-results"))

		if err != nil {
			return nil, fmt.Errorf("Invalid ?max-results= parameter, %w", err)
		}

		max_results = int32(v)
		slog.Debug("Reassign max results", "value", max_results)
	}

	if q.Has("compression") {

		compression = q.Get("compression")

		if !IsValidSQLiteCompression(compression) {
			return nil, fmt.Errorf("Invalid or unsupported compression")
		}
	}

	t := &SQLiteVec0Table{
		name:         "vec",
		dimensions:   dimensions,
		max_results:  max_results,
		max_distance: max_distance,
		compression:  compression,
	}

	return t, nil
}

func (t *SQLiteVec0Table) Name() string {
	return t.name
}

func (t *SQLiteVec0Table) Schema(*sql.DB) (string, error) {

	tp, err := template.ParseFS(sqlite_schema_fs, "sqlite_*_schema.txt")

	if err != nil {
		return "", err
	}

	switch t.compression {
	case sqlite_vec_quantize_compression:
		tp = tp.Lookup("sqlite_quantize")
	case sqlite_vec_matroyshka_compression:
		tp = tp.Lookup("sqlite_matroyshka")
	case sqlite_vec_default_compression:
		tp = tp.Lookup("sqlite_vec0")
	default:
		return "", fmt.Errorf("Invalid or unsupported compression '%s'", t.compression)
	}

	if tp == nil {
		return "", fmt.Errorf("Missing schema template")
	}

	vars := SQLiteVec0TableSchemaVars{
		Dimensions: t.dimensions,
		TableName:  t.Name(),
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

func (t *SQLiteVec0Table) InitializeTable(context.Context, *sql.DB) error {
	return nil
}

func (t *SQLiteVec0Table) IndexRecord(context.Context, *sql.DB, *sql.Tx, any) error {
	return fmt.Errorf("Not implemented")
}
