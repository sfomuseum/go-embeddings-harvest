package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/aaronland/go-pagination"
	"github.com/aaronland/go-pagination/countable"
	"github.com/aaronland/gocloud/runtimevar"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-embeddingsdb/database"
	embeddingsdb_grpc "github.com/sfomuseum/go-embeddingsdb/grpc"
	"github.com/sfomuseum/go-embeddingsdb/options"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
)

// GrpcClient implements the [Client] interface for a gRPC-based embeddings database.
type GrpcClient struct {
	Client
	conn   *grpc.ClientConn
	client embeddingsdb_grpc.EmbeddingsDBServiceClient
}

func init() {
	ctx := context.Background()
	RegisterClient(ctx, "grpc", NewGrpcClient)
}

// NewGrpcClient will return a new [GrpcClient] instance implementing the [Client] interface
// derived from 'uri' which is expected to take the port of:
//
//	grpc://{HOST}:{PORT}?{PARAMETERS}
//
// Where {PARAMETERS} may be one or more of the following:
// * `tls-certificate` – The path to a valid TLS certificate to use for encrypted connections.
// * `tls-ca-certificate` – The path to a custom TLS authority certificate to use for encrypted connections.
// * `tls-insecure` – Skip TLS verification steps. Use with caution.
// * `token-uri` – A registered `gocloud.dev/runtimevar` URI used to stored a shared authentication to require with client requests.
func NewGrpcClient(ctx context.Context, uri string) (Client, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, fmt.Errorf("Failed to parse URI, %w", err)
	}

	q := u.Query()

	q_tls_cert := q.Get("tls-certificate")
	q_tls_ca := q.Get("tls-ca-certificate")
	q_tls_insecure := q.Get("tls-insecure")

	addr := u.Host

	opts := make([]grpc.DialOption, 0)

	if q_tls_cert != "" {

		slog.Debug("Set up TLS")

		cert_pool := x509.NewCertPool()

		cert, err := os.ReadFile(q_tls_cert)

		if err != nil {
			return nil, fmt.Errorf("Failed to read certificate file, %w", err)
		}

		ok := cert_pool.AppendCertsFromPEM(cert)

		if !ok {
			return nil, fmt.Errorf("Failed to append certificate, %w", err)
		}

		if q_tls_ca != "" {

			ca_cert, err := ioutil.ReadFile(q_tls_ca)

			if err != nil {
				return nil, fmt.Errorf("Failed to create CA certificate, %w", err)
			}

			ok := cert_pool.AppendCertsFromPEM(ca_cert)

			if !ok {
				return nil, fmt.Errorf("Failed to append CA certificate, %w", err)
			}
		}

		tls_config := &tls.Config{
			RootCAs: cert_pool,
		}

		if q_tls_ca == "" && q_tls_insecure != "" {

			v, err := strconv.ParseBool(q_tls_insecure)

			if err != nil {
				return nil, fmt.Errorf("Failed to parse ?tls-insecure= parameter, %w", err)
			}

			tls_config.InsecureSkipVerify = v
		}

		tls_credentials := credentials.NewTLS(tls_config)
		opts = append(opts, grpc.WithTransportCredentials(tls_credentials))

	} else {
		slog.Debug("Allow insecure connections")
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if q.Has("token-uri") {

		slog.Debug("Set up token")

		token, err := runtimevar.StringVar(ctx, q.Get("token-uri"))

		if err != nil {
			return nil, fmt.Errorf("Failed to derive token, %w", err)
		}

		token = strings.TrimSpace(token)

		token_source := &oauth2.Token{
			AccessToken: token,
		}

		rpc_creds := oauth.TokenSource{
			TokenSource: oauth2.StaticTokenSource(token_source),
		}

		opts = append(opts, grpc.WithPerRPCCredentials(rpc_creds))
	}

	conn, err := grpc.NewClient(addr, opts...)

	if err != nil {
		return nil, fmt.Errorf("Failed to dial '%s', %w", addr, err)
	}

	client := embeddingsdb_grpc.NewEmbeddingsDBServiceClient(conn)

	e := &GrpcClient{
		conn:   conn,
		client: client,
	}

	return e, nil
}

// AddRecord adds 'record' to a gRPC-backed embeddings database.
func (e *GrpcClient) AddRecord(ctx context.Context, record *embeddingsdb.Record) error {

	grpc_record := embeddingsdb.EmbeddingsDBRecordToGrpcEmbeddingsDBRecord(record)

	req := &embeddingsdb_grpc.AddRecordRequest{
		Record: grpc_record,
	}

	_, err := e.client.AddRecord(ctx, req)

	if err != nil {
		return fmt.Errorf("Failed to add record, %w", err)
	}

	return nil
}

// GetRecord retrieves the record matching 'provider', 'depiction_id' and 'model' from a gRPC-backed embeddings database.
func (e *GrpcClient) GetRecord(ctx context.Context, req *embeddingsdb.GetRecordRequest, opts ...options.Option) (*embeddingsdb.Record, error) {

	grpc_req := &embeddingsdb_grpc.GetRecordRequest{
		Provider:    req.Provider,
		DepictionId: req.DepictionId,
		Model:       req.Model,
	}

	// opts here...

	rsp, err := e.client.GetRecord(ctx, grpc_req)

	if err != nil {
		return nil, fmt.Errorf("Failed to derive embeddings, %w", err)
	}

	record := embeddingsdb.GrpcEmbeddingsRecordToEmbeddingsDBRecord(rsp.Record)

	return record, nil
}

func (e *GrpcClient) RemoveRecord(ctx context.Context, req *embeddingsdb.RemoveRecordRequest, opts ...options.Option) error {

	grpc_req := &embeddingsdb_grpc.RemoveRecordRequest{
		Provider:    req.Provider,
		DepictionId: req.DepictionId,
		Model:       req.Model,
	}

	// opts here...

	_, err := e.client.RemoveRecord(ctx, grpc_req)

	if err != nil {
		return fmt.Errorf("Failed to remove record, %w", err)
	}

	return nil
}

func (e *GrpcClient) ListRecords(ctx context.Context, pg_opts pagination.Options, opts ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error) {

	grpc_pg := &embeddingsdb_grpc.PaginationOptions{
		Page:    countable.PageFromOptions(pg_opts),
		PerPage: pg_opts.PerPage(),
	}

	grpc_req := &embeddingsdb_grpc.ListRecordsRequest{
		Pagination: grpc_pg,
	}

	// opts here...

	grpc_rsp, err := e.client.ListRecords(ctx, grpc_req)

	if err != nil {
		return nil, nil, err
	}

	records := make([]*embeddingsdb.Record, len(grpc_rsp.Records))

	for i, grpc_r := range grpc_rsp.Records {
		records[i] = embeddingsdb.GrpcEmbeddingsRecordToEmbeddingsDBRecord(grpc_r)
	}

	pg_rsp, err := countable.NewResultsFromCountWithOptions(pg_opts, grpc_rsp.Pagination.Total)

	if err != nil {
		return nil, nil, err
	}

	return records, pg_rsp, nil
}

// SimilarRecords retrieves records with embeddings similar to those defined in 'req' from a gRPC-backed embeddings database.
func (e *GrpcClient) SimilarRecords(ctx context.Context, req *embeddingsdb.SimilarRecordsRequest, opts ...options.Option) ([]*embeddingsdb.SimilarRecord, error) {

	grpc_req := &embeddingsdb_grpc.SimilarRecordsRequest{
		Model:           req.Model,
		Embeddings:      req.Embeddings,
		SimilarProvider: req.SimilarProvider,
		MaxResults:      req.MaxResults,
		MaxDistance:     req.MaxDistance,
		Exclude:         req.Exclude,
	}

	rsp, err := e.client.SimilarRecords(ctx, grpc_req)

	if err != nil {
		return nil, fmt.Errorf("Failed to query records, %w", err)
	}

	results := embeddingsdb.GrpcSimilarRecordsResultsToEmbeddingDBSimilarRecords(rsp.Records)
	return results, nil
}

// SimilarRecordsById retrieves records with embeddings similar to those for the record matching 'provider', 'depiction_id' and 'model' from a gRPC-backed embeddings database.
func (e *GrpcClient) SimilarRecordsById(ctx context.Context, req *embeddingsdb.SimilarRecordsByIdRequest, opts ...options.Option) ([]*embeddingsdb.SimilarRecord, error) {

	grpc_req := &embeddingsdb_grpc.SimilarRecordsByIdRequest{
		Provider:        req.Provider,
		DepictionId:     req.DepictionId,
		Model:           req.Model,
		SimilarProvider: req.SimilarProvider,
		MaxResults:      req.MaxResults,
		MaxDistance:     req.MaxDistance,
	}

	rsp, err := e.client.SimilarRecordsById(ctx, grpc_req)

	if err != nil {
		return nil, fmt.Errorf("Failed to query records, %w", err)
	}

	results := embeddingsdb.GrpcSimilarRecordsResultsToEmbeddingDBSimilarRecords(rsp.Records)
	return results, nil
}

func (e *GrpcClient) Models(ctx context.Context, opts ...options.Option) ([]string, error) {

	req := &embeddingsdb_grpc.GetModelsRequest{
		// 		Provider: providers,
	}

	// opts here...

	rsp, err := e.client.GetModels(ctx, req)

	if err != nil {
		return nil, fmt.Errorf("Failed to list models, %w", err)
	}

	return rsp.Model, nil
}

func (e *GrpcClient) Providers(ctx context.Context, opts ...options.Option) ([]string, error) {

	req := &embeddingsdb_grpc.GetProvidersRequest{}

	// opts here...

	rsp, err := e.client.GetProviders(ctx, req)

	if err != nil {
		return nil, fmt.Errorf("Failed to list providers, %w", err)
	}

	return rsp.Provider, nil
}

func (cl *GrpcClient) PaginationType(ctx context.Context, opts ...options.Option) (database.PaginationType, error) {

	req := &embeddingsdb_grpc.GetPaginationTypeRequest{}

	// opts here...

	rsp, err := cl.client.GetPaginationType(ctx, req)

	if err != nil {
		return database.NullPaginationType, fmt.Errorf("Failed to determine pagination type, %w", err)
	}

	return database.NewPaginationType(rsp.PaginationType)
}

func (cl *GrpcClient) Close(ctx context.Context) error {
	return nil
}
