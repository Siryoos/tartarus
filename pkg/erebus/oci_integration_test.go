package erebus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOCIBuilder_BuildRootFS(t *testing.T) {
	// Create a temporary directory for source files
	srcDir, err := os.MkdirTemp("", "erebus-rootfs-src")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)

	// Create some files in the source directory
	err = os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("hello world"), 0644)
	require.NoError(t, err)
	err = os.Mkdir(filepath.Join(srcDir, "subdir"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "subdir", "foo.txt"), []byte("bar"), 0644)
	require.NoError(t, err)

	// Create a temporary directory for output
	outDir, err := os.MkdirTemp("", "erebus-rootfs-out")
	require.NoError(t, err)
	defer os.RemoveAll(outDir)

	dstFile := filepath.Join(outDir, "rootfs.ext4")

	builder := NewOCIBuilder(nil, nil)

	err = builder.BuildRootFS(context.Background(), srcDir, dstFile)

	if errors.Is(err, ErrToolNotFound) {
		t.Skip("genext2fs not found, skipping integration test")
	}

	require.NoError(t, err)

	// Verify the output file exists and has size > 0
	info, err := os.Stat(dstFile)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))

	// Ideally we would mount it and check contents, but that requires root/sudo usually.
	// We trust genext2fs if it returned success.
}
