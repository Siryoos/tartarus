package erebus

import (
	"context"
	"io"
)

// Store is Erebus: the deep gloom blob store for images & snapshots.

type Store interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Exists(ctx context.Context, key string) (bool, error)
	Delete(ctx context.Context, key string) error
}
