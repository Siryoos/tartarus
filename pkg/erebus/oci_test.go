package erebus

import (
	"archive/tar"
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
	"github.com/stretchr/testify/require"
)

func TestOCIBuilder_InjectInit_CustomPath(t *testing.T) {
	// Create a temporary init binary
	tempDir := t.TempDir()
	customInitPath := filepath.Join(tempDir, "custom-init")
	err := os.WriteFile(customInitPath, []byte("#!/bin/sh\necho custom init"), 0755)
	require.NoError(t, err)

	// Create output directory
	outputDir := t.TempDir()

	// Initialize builder with custom init path
	cacheDir := t.TempDir()
	store, err := NewLocalStore(cacheDir)
	require.NoError(t, err)

	builder := NewOCIBuilder(store, nil)
	builder.InitPath = customInitPath

	// Inject
	err = builder.InjectInit(context.Background(), outputDir)
	require.NoError(t, err)

	// Verify content
	injectedPath := filepath.Join(outputDir, "init")
	require.FileExists(t, injectedPath)

	content, err := os.ReadFile(injectedPath)
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\necho custom init", string(content))
}

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

	cacheDir, err := os.MkdirTemp("", "erebus-oci-cache")
	assert.NoError(t, err)
	defer os.RemoveAll(cacheDir)

	store, err := NewLocalStore(cacheDir)
	assert.NoError(t, err)

	logger := new(MockLogger)
	logger.On("Info", mock.Anything, mock.Anything, mock.Anything).Return()
	logger.On("Error", mock.Anything, mock.Anything, mock.Anything).Return()

	builder := NewOCIBuilder(store, logger)
	// Use mock scanner to avoid external dependency
	scanner := new(TestMockScanner)
	scanner.On("Scan", mock.Anything, mock.Anything).Return(nil)
	builder.Scanner = scanner

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
	// We don't need to read it into bytes, just pass the reader to Put
	// But Put consumes the reader, so we need to be careful if we need it again.
	// In this test, we just Put it once.

	cacheDir, err := os.MkdirTemp("", "erebus-oci-cache-hit")
	assert.NoError(t, err)
	defer os.RemoveAll(cacheDir)

	store, err := NewLocalStore(cacheDir)
	assert.NoError(t, err)

	// Pre-populate cache
	digest, err := layer.Digest()
	assert.NoError(t, err)
	key := "layers/" + digest.Hex
	err = store.Put(context.Background(), key, compressed)
	assert.NoError(t, err)

	logger := new(MockLogger)
	logger.On("Info", mock.Anything, mock.Anything, mock.Anything).Return()

	builder := NewOCIBuilder(store, logger)
	// Use mock scanner to avoid external dependency
	scanner := new(TestMockScanner)
	scanner.On("Scan", mock.Anything, mock.Anything).Return(nil)
	builder.Scanner = scanner

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

// TestMockScanner for testing
type TestMockScanner struct {
	mock.Mock
}

func (m *TestMockScanner) Scan(ctx context.Context, path string) error {
	args := m.Called(ctx, path)
	return args.Error(0)
}

func TestOCIBuilder_Assemble_Scan(t *testing.T) {
	// Create a layer with a single file
	content := []byte("hello scan")
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		pr, pw := io.Pipe()
		go func() {
			tw := tar.NewWriter(pw)
			hdr := &tar.Header{
				Name: "scan.txt",
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

	cacheDir, err := os.MkdirTemp("", "erebus-oci-scan-cache")
	assert.NoError(t, err)
	defer os.RemoveAll(cacheDir)

	store, err := NewLocalStore(cacheDir)
	assert.NoError(t, err)

	logger := new(MockLogger)
	logger.On("Info", mock.Anything, mock.Anything, mock.Anything).Return()

	scanner := new(TestMockScanner)
	scanner.On("Scan", mock.Anything, mock.Anything).Return(nil)

	builder := NewOCIBuilder(store, logger)
	builder.Scanner = scanner
	builder.Fetcher = func(ctx context.Context, ref string) (v1.Image, error) {
		return img, nil
	}

	tmpDir, err := os.MkdirTemp("", "erebus-oci-scan-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	err = builder.Assemble(context.Background(), "fake.registry/repo:tag", tmpDir)
	assert.NoError(t, err)

	scanner.AssertExpectations(t)
}
