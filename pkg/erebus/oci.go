package erebus

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// ImageFetcher is a function that fetches an image.
type ImageFetcher func(ctx context.Context, ref string) (v1.Image, error)

// OCIBuilder handles pulling and extracting OCI images.
type OCIBuilder struct {
	Store   Store
	Logger  hermes.Logger
	Fetcher ImageFetcher
	Scanner Scanner
}

// NewOCIBuilder creates a new OCIBuilder.
func NewOCIBuilder(store Store, logger hermes.Logger) *OCIBuilder {
	return &OCIBuilder{
		Store:   store,
		Logger:  logger,
		Scanner: NewTrivyScanner(),
		Fetcher: func(ctx context.Context, ref string) (v1.Image, error) {
			nameRef, err := name.ParseReference(ref)
			if err != nil {
				return nil, fmt.Errorf("parsing reference %q: %w", ref, err)
			}
			return remote.Image(nameRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
		},
	}
}

// Pull pulls an image from a registry.
func (b *OCIBuilder) Pull(ctx context.Context, ref string) (v1.Image, error) {
	if b.Logger != nil {
		b.Logger.Info(ctx, "Pulling OCI image", map[string]any{"ref": ref})
	}

	return b.Fetcher(ctx, ref)
}

// Assemble pulls the image and extracts its layers to the output directory.
func (b *OCIBuilder) Assemble(ctx context.Context, ref string, outputDir string) error {
	img, err := b.Pull(ctx, ref)
	if err != nil {
		return err
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	if b.Logger != nil {
		b.Logger.Info(ctx, "Extracting layers", map[string]any{"count": len(layers), "output_dir": outputDir})
	}

	// Apply layers in order
	for i, layer := range layers {
		digest, err := layer.Digest()
		if err != nil {
			return fmt.Errorf("getting layer digest: %w", err)
		}

		if b.Logger != nil {
			b.Logger.Info(ctx, "Processing layer", map[string]any{"index": i, "digest": digest.String()})
		}

		// Check if we have the layer cached in Store
		key := fmt.Sprintf("layers/%s", digest.Hex)
		exists, err := b.Store.Exists(ctx, key)
		if err != nil {
			// Log error but continue? Or fail?
			// For now, fail as it might indicate store issues.
			return fmt.Errorf("checking layer cache: %w", err)
		}

		var rc io.ReadCloser
		if exists {
			if b.Logger != nil {
				b.Logger.Info(ctx, "Cache hit for layer", map[string]any{"digest": digest.String()})
			}
			cached, err := b.Store.Get(ctx, key)
			if err != nil {
				return fmt.Errorf("getting cached layer: %w", err)
			}
			rc = cached
		} else {
			if b.Logger != nil {
				b.Logger.Info(ctx, "Cache miss for layer, downloading", map[string]any{"digest": digest.String()})
			}
			// Get compressed stream
			compressed, err := layer.Compressed()
			if err != nil {
				return fmt.Errorf("getting compressed layer content: %w", err)
			}

			// Tee to store
			pr, pw := io.Pipe()
			go func() {
				if err := b.Store.Put(ctx, key, pr); err != nil {
					if b.Logger != nil {
						b.Logger.Error(ctx, "Failed to cache layer", map[string]any{"digest": digest.String(), "error": err.Error()})
					}
					pr.CloseWithError(err) // This will cause the reader to fail
				} else {
					pr.Close()
				}
			}()

			// Wrap compressed with TeeReader to write to pw
			// We need to close pw when we are done reading from the TeeReader
			rc = &readCloserWrapper{
				Reader: io.TeeReader(compressed, pw),
				Closer: func() error {
					pw.Close()
					return compressed.Close()
				},
			}
		}

		// Decompress
		gzipReader, err := gzip.NewReader(rc)
		if err != nil {
			rc.Close()
			return fmt.Errorf("creating gzip reader: %w", err)
		}
		defer gzipReader.Close()
		defer rc.Close()

		if err := untar(gzipReader, outputDir); err != nil {
			return fmt.Errorf("extracting layer %s: %w", digest, err)
		}
	}

	return nil
}

type readCloserWrapper struct {
	io.Reader
	Closer func() error
}

func (w *readCloserWrapper) Close() error {
	return w.Closer()
}

// BuildRootFS converts a directory to a rootfs disk image.
// This currently requires external tools like genext2fs or virt-make-fs.
func (b *OCIBuilder) BuildRootFS(ctx context.Context, srcDir, dstFile string) error {
	if b.Logger != nil {
		b.Logger.Info(ctx, "Building rootfs image", map[string]any{"src": srcDir, "dst": dstFile})
	}

	// TODO: Implement actual disk creation.
	// For now, we just create a dummy file to satisfy the interface if we are in a test/dev environment without tools.
	// In production, this should use a tool to create an ext4 image.

	// Check for genext2fs
	// cmd := exec.CommandContext(ctx, "genext2fs", "-b", "1024", "-d", srcDir, dstFile)
	// ...

	// Fallback: create a dummy file
	f, err := os.Create(dstFile)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write some bytes
	if _, err := f.WriteString("dummy-rootfs"); err != nil {
		return err
	}

	return nil
}

// untar extracts a tar stream to a destination directory.
func untar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)

		// Sanitize path to prevent Zip Slip (though unlikely with standard OCI images, good practice)
		if !filepath.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", target)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			// For now, just create the symlink.
			// Note: This might fail if the target doesn't exist yet, but standard tar behavior handles this.
			// We might need to handle hardlinks too.
			if err := os.Symlink(header.Linkname, target); err != nil {
				// Ignore existence errors for now? No, symlink creation shouldn't fail if target is missing.
				// But if the file already exists (e.g. overwritten by a later layer?), we should remove it first.
				os.Remove(target)
				if err := os.Symlink(header.Linkname, target); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
