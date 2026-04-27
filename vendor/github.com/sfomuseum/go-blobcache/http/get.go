package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	net_http "net/http"

	"github.com/sfomuseum/go-blobcache"
	"github.com/whosonfirst/go-ioutil"
)

var http_cl = net_http.Client{}

func GetWithCache(ctx context.Context, c *blobcache.BlobCache, uri string) (io.ReadSeekCloser, error) {

	logger := slog.Default()
	logger = logger.With("uri", uri)

	r, err := c.Get(ctx, uri)

	if err != nil && err != blobcache.CacheMiss {
		logger.Debug("Failed to retrieve from cache", "error", err)
		return nil, err
	}

	if err != nil && err == blobcache.CacheMiss {
		logger.Debug("Cache miss, fetch from source")
		return getWithCache(ctx, c, uri)
	}

	// This is not working as expected yet

	/*
		etag, err := c.ETag(ctx, uri)

		if err != nil {
			logger.Debug("Failed to determine etag, fetch from source", "error", err)
			return getWithCache(ctx, c, uri)
		}

		req, err := net_http.NewRequestWithContext(ctx, net_http.MethodGet, uri, nil)

		if err != nil {
			logger.Debug("Failed to create request, fetch from source", "error", err)
			return getWithCache(ctx, c, uri)
		}

		logger.Debug("Query with etag", "etag", etag)
		req.Header.Set("If-None-Match", etag)

		rsp, err := http_cl.Do(req)

		if err != nil {
			logger.Debug("Failed to execute request, fetch from source", "error", err)
			return getWithCache(ctx, c, uri)
		}

		if rsp.StatusCode != net_http.StatusNotModified {
			logger.Debug("status code not not-modified, fetch from source", "code", rsp.StatusCode)
			return getWithCache(ctx, c, uri)
		}
	*/

	logger.Debug("Return from cache")
	return r, nil
}

func getWithCache(ctx context.Context, c *blobcache.BlobCache, uri string) (io.ReadSeekCloser, error) {

	logger := slog.Default()
	logger = logger.With("uri", uri)

	req, err := net_http.NewRequestWithContext(ctx, net_http.MethodGet, uri, nil)

	if err != nil {
		return nil, err
	}

	rsp, err := http_cl.Do(req)

	if err != nil {
		return nil, err
	}

	defer rsp.Body.Close()

	if rsp.StatusCode != net_http.StatusOK {
		return nil, fmt.Errorf("Request failed %s", rsp.Status)
	}

	body, err := io.ReadAll(rsp.Body)

	if err != nil {
		return nil, err
	}

	br := bytes.NewReader(body)
	rsc, err := ioutil.NewReadSeekCloser(br)

	if err != nil {
		return nil, err
	}

	err = c.Set(ctx, uri, rsc)

	if err != nil {
		return nil, err
	}

	_, err = rsc.Seek(0, 0)

	if err != nil {
		return nil, err
	}

	logger.Debug("Return from source")
	return rsc, nil
}
