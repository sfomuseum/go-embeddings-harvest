package main

// go run cmd/harvest-embeddings/main.go -harvest-uri cma:///usr/local/data/cma/openaccess/data.csv -model s1 -output work/cma.parquet -verbose

import (
	"context"
	"log"

	"github.com/sfomuseum/go-embeddings-harvest/app/harvest"
)

func main() {

	ctx := context.Background()
	err := harvest.Run(ctx)

	if err != nil {
		log.Fatalf("Failed to harvest embeddings, %v", err)
	}
}
