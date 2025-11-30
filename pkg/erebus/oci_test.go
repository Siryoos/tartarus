package erebus

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockStore is a mock implementation of Store.
type MockStore struct {
	mock.Mock
}

func (m *MockStore) Put(ctx context.Context, key string, r io.Reader) error {
	args := m.Called(ctx, key, r)
	return args.Error(0)
}

func (m *MockStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockStore) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

func (m *MockStore) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

// MockLogger is a mock implementation of hermes.Logger.
type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Info(ctx context.Context, msg string, fields map[string]any) {
	m.Called(ctx, msg, fields)
}

func (m *MockLogger) Error(ctx context.Context, msg string, fields map[string]any) {
	m.Called(ctx, msg, fields)
}

func TestOCIBuilder_Assemble(t *testing.T) {
	// Create a layer with a single file
	content := []byte("hello world")
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		pr, pw := io.Pipe()
		go func() {
			tw := tar.NewWriter(pw)
			hdr := &tar.Header{
				Name: "hello.txt",
				Mode: 0644,
				Size: int64(len(content)),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				pw.CloseWithError(err)
				return
			}
			if _, err := tw.Write(content); err != nil {
				pw.CloseWithError(err)
				return
			}
			tw.Close()
			pw.Close()
		}()
		return pr, nil
	})
	assert.NoError(t, err)

	img, err := mutate.AppendLayers(empty.Image, layer)
	assert.NoError(t, err)

	store := new(MockStore)
	store.On("Exists", mock.Anything, mock.Anything).Return(false, nil)
	store.On("Put", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		r := args.Get(2).(io.Reader)
		io.Copy(io.Discard, r)
	}).Return(nil)

	logger := new(MockLogger)
	logger.On("Info", mock.Anything, mock.Anything, mock.Anything).Return()
	logger.On("Error", mock.Anything, mock.Anything, mock.Anything).Return()

	builder := NewOCIBuilder(store, logger)
	builder.Fetcher = func(ctx context.Context, ref string) (v1.Image, error) {
		return img, nil
	}

	tmpDir, err := os.MkdirTemp("", "erebus-oci-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	err = builder.Assemble(context.Background(), "fake.registry/repo:tag", tmpDir)
	assert.NoError(t, err)

	// Verify file exists
	data, err := os.ReadFile(filepath.Join(tmpDir, "hello.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestOCIBuilder_Assemble_CacheHit(t *testing.T) {
	// Create a layer with a single file
	content := []byte("hello cache")
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		pr, pw := io.Pipe()
		go func() {
			tw := tar.NewWriter(pw)
			hdr := &tar.Header{
				Name: "cached.txt",
				Mode: 0644,
				Size: int64(len(content)),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				pw.CloseWithError(err)
				return
			}
			if _, err := tw.Write(content); err != nil {
				pw.CloseWithError(err)
				return
			}
			tw.Close()
			pw.Close()
		}()
		return pr, nil
	})
	assert.NoError(t, err)

	img, err := mutate.AppendLayers(empty.Image, layer)
	assert.NoError(t, err)

	// Get the compressed content of the layer to serve from mock store
	compressed, err := layer.Compressed()
	assert.NoError(t, err)
	compressedBytes, err := io.ReadAll(compressed)
	assert.NoError(t, err)
	compressed.Close()

	store := new(MockStore)
	store.On("Exists", mock.Anything, mock.Anything).Return(true, nil)
	store.On("Get", mock.Anything, mock.Anything).Return(io.NopCloser(bytes.NewReader(compressedBytes)), nil)

	logger := new(MockLogger)
	logger.On("Info", mock.Anything, mock.Anything, mock.Anything).Return()

	builder := NewOCIBuilder(store, logger)
	builder.Fetcher = func(ctx context.Context, ref string) (v1.Image, error) {
		return img, nil
	}

	tmpDir, err := os.MkdirTemp("", "erebus-oci-test-hit")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	err = builder.Assemble(context.Background(), "fake.registry/repo:tag", tmpDir)
	assert.NoError(t, err)

	// Verify file exists
	data, err := os.ReadFile(filepath.Join(tmpDir, "cached.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "hello cache", string(data))
}
