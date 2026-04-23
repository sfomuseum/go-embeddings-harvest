//go:build bleve

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

// BleveEmbeddingsTable stores serialized vector embeddings associated
// with records in a Bleve database. This is necessary because vector
// embeddings are not returned by default in Bleve's "Fields" dictionary
// and storing them internally (in Bleve) incurs a prohibitive disk
// usage cost.
type BleveEmbeddingsTable struct {
	sfom_sql.Table
	name string
}

type BleveEmbeddingsTableSchemaVars struct {
	TableName string
}

func NewBleveEmbeddingsTable(ctx context.Context, uri string) (sfom_sql.Table, error) {

	t := &BleveEmbeddingsTable{
		name: "embeddings",
	}

	return t, nil
}

func (t *BleveEmbeddingsTable) Name() string {
	return t.name
}

func (t *BleveEmbeddingsTable) Schema(*sql.DB) (string, error) {

	tp, err := template.ParseFS(bleve_schema_fs, "bleve_*_schema.txt")

	if err != nil {
		return "", err
	}

	tp = tp.Lookup("bleve_embeddings")

	if tp == nil {
		return "", fmt.Errorf("Missing schema template")
	}

	vars := BleveEmbeddingsTableSchemaVars{
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

func (t *BleveEmbeddingsTable) InitializeTable(context.Context, *sql.DB) error {
	return nil
}

func (t *BleveEmbeddingsTable) IndexRecord(context.Context, *sql.DB, *sql.Tx, any) error {
	return fmt.Errorf("Not implemented")
}
