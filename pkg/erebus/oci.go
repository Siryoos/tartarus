package erebus

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"golang.org/x/sync/errgroup"
)

// ImageFetcher is a function that fetches an image.
type ImageFetcher func(ctx context.Context, ref string) (v1.Image, error)

// ErrToolNotFound is returned when a required external tool is missing.
var ErrToolNotFound = errors.New("tool not found")

// OCIBuilder handles pulling and extracting OCI images.
type OCIBuilder struct {
	Store    Store
	Logger   hermes.Logger
	Fetcher  ImageFetcher
	Scanner  Scanner
	InitPath string
}

// NewOCIBuilder creates a new OCIBuilder.
func NewOCIBuilder(store Store, logger hermes.Logger) *OCIBuilder {
	return &OCIBuilder{
		Store:    store,
		Logger:   logger,
		Scanner:  NewTrivyScanner(),
		InitPath: "init", // Default
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

	// Prepare layer cache in parallel
	// We use a map to store the ready status of each layer
	// But we need to extract them in order.
	// So we'll trigger downloads for all, and then wait for them in order.

	type layerResult struct {
		index int
		err   error
	}

	// WaitGroup to wait for all downloads (for cleanup/error reporting)
	// But for extraction, we just need to know if Layer I is ready.

	// Let's use a slice of channels, one for each layer.
	// When layer I is downloaded, we close its channel or send a signal.
	layerReady := make([]chan error, len(layers))
	for i := range layers {
		layerReady[i] = make(chan error, 1)
	}

	g, ctx := errgroup.WithContext(ctx)

	// Start downloads
	for i, layer := range layers {
		i := i
		layer := layer
		g.Go(func() error {
			digest, err := layer.Digest()
			if err != nil {
				layerReady[i] <- err
				return err
			}

			key := fmt.Sprintf("layers/%s", digest.Hex)

			// Check if exists
			exists, err := b.Store.Exists(ctx, key)
			if err != nil {
				layerReady[i] <- err
				return err
			}

			if !exists {
				if b.Logger != nil {
					b.Logger.Info(ctx, "Cache miss for layer, downloading", map[string]any{"digest": digest.String()})
				}
				compressed, err := layer.Compressed()
				if err != nil {
					layerReady[i] <- err
					return err
				}
				defer compressed.Close()

				if err := b.Store.Put(ctx, key, compressed); err != nil {
					layerReady[i] <- err
					return err
				}
			} else {
				if b.Logger != nil {
					b.Logger.Info(ctx, "Cache hit for layer", map[string]any{"digest": digest.String()})
				}
			}

			// Signal ready
			layerReady[i] <- nil
			return nil
		})
	}

	// Start extraction loop (sequential)
	// We run this in the main goroutine (or another one, but we need to block Assemble anyway)
	// We wait for layer 0, extract, then layer 1, etc.

	extractErr := func() error {
		for i, layer := range layers {
			// Wait for download
			select {
			case err := <-layerReady[i]:
				if err != nil {
					return fmt.Errorf("layer %d download failed: %w", i, err)
				}
			case <-ctx.Done():
				return ctx.Err()
			}

			// Now extract
			digest, _ := layer.Digest() // Should be cached now
			key := fmt.Sprintf("layers/%s", digest.Hex)

			rc, err := b.Store.Get(ctx, key)
			if err != nil {
				return fmt.Errorf("getting cached layer %s: %w", digest, err)
			}

			// Decompress
			gzipReader, err := gzip.NewReader(rc)
			if err != nil {
				rc.Close()
				return fmt.Errorf("creating gzip reader: %w", err)
			}

			err = untar(gzipReader, outputDir)
			gzipReader.Close()
			rc.Close()

			if err != nil {
				return fmt.Errorf("extracting layer %s: %w", digest, err)
			}

			if b.Logger != nil {
				b.Logger.Info(ctx, "Extracted layer", map[string]any{"index": i, "digest": digest.String()})
			}
		}
		return nil
	}()

	// Wait for downloads to finish (should be done or cancelled)
	if err := g.Wait(); err != nil {
		// If download failed, extractErr likely already caught it or will be cancelled
		// But we return the first error
		if extractErr == nil {
			return err
		}
	}

	if extractErr != nil {
		return extractErr
	}

	// Inject Init
	if err := b.InjectInit(ctx, outputDir); err != nil {
		return fmt.Errorf("injecting init: %w", err)
	}

	// Scan the extracted directory
	if b.Scanner != nil {
		if err := b.Scanner.Scan(ctx, outputDir); err != nil {
			return fmt.Errorf("scanning extracted image: %w", err)
		}
	}

	return nil
}

// InjectInit injects the init binary into the rootfs.
func (b *OCIBuilder) InjectInit(ctx context.Context, outputDir string) error {
	// List of potential paths to check
	candidates := []string{}

	// 1. Configured path (or default "init")
	if b.InitPath != "" {
		candidates = append(candidates, b.InitPath)
	} else {
		candidates = append(candidates, "init")
	}

	// 2. Path relative to the current executable
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "init"))
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "tartarus-init"))
	}

	// 3. Common system paths
	candidates = append(candidates, "/usr/local/bin/tartarus-init")
	candidates = append(candidates, "/opt/tartarus/bin/init")

	// 4. LookPath
	if path, err := exec.LookPath("tartarus-init"); err == nil {
		candidates = append(candidates, path)
	}

	var foundPath string
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			foundPath = path
			if b.Logger != nil {
				b.Logger.Info(ctx, "Found init binary", map[string]any{"path": path})
			}
			break
		}
	}

	if foundPath == "" {
		if b.Logger != nil {
			b.Logger.Info(ctx, "Init binary not found in any candidate locations, skipping injection", map[string]any{"candidates": candidates, "level": "warn"})
		}
		return nil
	}

	dest := filepath.Join(outputDir, "init")
	srcFile, err := os.Open(foundPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
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
// This currently requires external tools like genext2fs.
func (b *OCIBuilder) BuildRootFS(ctx context.Context, srcDir, dstFile string) error {
	if b.Logger != nil {
		b.Logger.Info(ctx, "Building rootfs image", map[string]any{"src": srcDir, "dst": dstFile})
	}

	// Check for genext2fs
	genext2fsPath, err := exec.LookPath("genext2fs")
	if err != nil {
		return fmt.Errorf("%w: genext2fs", ErrToolNotFound)
	}

	// Calculate directory size to determine image size
	// We'll add some overhead (e.g. 10%) + fixed buffer (e.g. 10MB) to be safe
	dirSize, err := calculateDirSize(srcDir)
	if err != nil {
		return fmt.Errorf("calculating directory size: %w", err)
	}

	// Convert to KB for genext2fs -b
	// Size = dirSize * 1.1 + 10MB
	sizeKB := (dirSize*11/10)/1024 + 10240

	// genext2fs -b <blocks_in_kb> -d <src> <dst>
	cmd := exec.CommandContext(ctx, genext2fsPath, "-b", fmt.Sprintf("%d", sizeKB), "-d", srcDir, dstFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("genext2fs failed: %w, output: %s", err, string(output))
	}

	return nil
}

func calculateDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// untar extracts a tar stream to a destination directory.
func untar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)

	cleanDest := filepath.Clean(dest) + string(os.PathSeparator)

	// Cache created directories to avoid MkdirAll calls
	createdDirs := make(map[string]bool)
	createdDirs[dest] = true
	createdDirs[filepath.Clean(dest)] = true

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)

		// Optimized Zip Slip check
		// filepath.Join calls Clean, so target is clean.
		// We just need to check prefix.
		if target != filepath.Clean(dest) && !strings.HasPrefix(target, cleanDest) {
			return fmt.Errorf("illegal file path: %s", target)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if createdDirs[target] {
				continue
			}
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			createdDirs[target] = true
		case tar.TypeReg:
			// Ensure parent dir exists
			dir := filepath.Dir(target)
			if !createdDirs[dir] {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
				createdDirs[dir] = true
			}

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
			// Ensure parent dir exists
			dir := filepath.Dir(target)
			if !createdDirs[dir] {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
				createdDirs[dir] = true
			}

			if err := os.Symlink(header.Linkname, target); err != nil {
				os.Remove(target)
				if err := os.Symlink(header.Linkname, target); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
