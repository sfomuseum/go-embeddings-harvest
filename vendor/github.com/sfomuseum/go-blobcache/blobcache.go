package blobcache

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/aaronland/gocloud/blob/bucket"
	"gocloud.dev/blob"
)

var CacheMiss = errors.New("Cache miss")

type BlobCache struct {
	bucket *blob.Bucket
}

func NewBlobCache(ctx context.Context, uri string) (*BlobCache, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	var b *blob.Bucket

	switch u.Scheme {
	case "null":
		//
	default:

		switch u.Scheme {
		case "file":
			
			err := os.MkdirAll(u.Path, 0750)
			
			if err != nil {
				return nil, err
			}
		}
		
		b, err = bucket.OpenBucket(ctx, uri)

		if err != nil {
			return nil, err
		}

	}

	c := &BlobCache{
		bucket: b,
	}

	return c, nil

}

func (c *BlobCache) Get(ctx context.Context, key string) (io.ReadSeekCloser, error) {

	if c.bucket == nil {
		return nil, CacheMiss
	}

	path := c.derivePathFromKey(key)

	exists, err := c.bucket.Exists(ctx, path)

	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, CacheMiss
	}

	return c.bucket.NewReader(ctx, path, nil)
}

func (c *BlobCache) LastModified(ctx context.Context, key string) (time.Time, error) {

	if c.bucket == nil {
		return time.Time{}, CacheMiss
	}

	attrs, err := c.attributes(ctx, key)

	if err != nil {
		return time.Time{}, err
	}

	return attrs.ModTime, nil
}

func (c *BlobCache) ETag(ctx context.Context, key string) (string, error) {

	if c.bucket == nil {
		return "", CacheMiss
	}

	attrs, err := c.attributes(ctx, key)

	if err != nil {
		return "", err
	}

	return attrs.ETag, nil
}

func (c *BlobCache) Set(ctx context.Context, key string, r io.Reader) error {

	if c.bucket == nil {
		return nil
	}

	path := c.derivePathFromKey(key)
	wr, err := c.bucket.NewWriter(ctx, path, nil)

	if err != nil {
		return err
	}

	_, err = io.Copy(wr, r)

	if err != nil {
		return err
	}

	return wr.Close()
}

func (c *BlobCache) Unset(ctx context.Context, key string) error {

	if c.bucket == nil {
		return nil
	}

	path := c.derivePathFromKey(key)
	return c.bucket.Delete(ctx, path)
}

func (c *BlobCache) Close() error {

	if c.bucket == nil {
		return nil
	}

	return c.bucket.Close()
}

func (c *BlobCache) attributes(ctx context.Context, key string) (*blob.Attributes, error) {

	path := c.derivePathFromKey(key)

	exists, err := c.bucket.Exists(ctx, path)

	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, CacheMiss
	}

	return c.bucket.Attributes(ctx, path)
}

func (c *BlobCache) derivePathFromKey(key string) string {

	path := c.deriveTreeFromKey(key)

	u, err := url.Parse(key)

	if err == nil {
		path = filepath.Join(u.Host, path)
	}

	return path
}

func (c *BlobCache) deriveTreeFromKey(key string) string {

	input := c.hashKey(key)
	parts := []string{}

	for len(input) > 3 {

		chunk := input[0:3]
		input = input[3:]
		parts = append(parts, chunk)
	}

	if len(input) > 0 {
		parts = append(parts, input)
	}

	path := filepath.Join(parts...)
	return path
}

func (c *BlobCache) hashKey(key string) string {

	data := []byte(key)
	return fmt.Sprintf("%x", md5.Sum(data))
}
