// Package erebus implements the deep artifact store — the Primordial Darkness
// that holds every container image and WASM module before it descends into
// the execution runtime.
//
// Erebus pulls OCI images from a registry, extracts their layers into a local
// content-addressable cache, and exposes a read-only root filesystem to
// Firecracker/gVisor/WASM runtimes. It also supports pushing snapshots to
// remote object storage for cross-node sharing.
//
// # Implementations
//
//   - [LocalStore]: Content-addressable cache on the node's local filesystem.
//     Used as the primary cache for extracted image layers.
//
//   - [S3Store]: Remote store backed by any S3-compatible API (AWS S3,
//     MinIO, GCS via compatibility layer). Used to share snapshots across
//     nodes and as long-term artifact archival.
//
//   - [OCIStore]: Pulls images from an OCI registry, verifies digests, and
//     populates the LocalStore. Performs a dynamic search for the tartarus
//     init binary with configurable fallback paths.
//
//   - [Scanner]: Scans an extracted rootfs for known CVEs using Trivy or a
//     compatible vulnerability database.
//
// # Basic Usage
//
//	local, err := erebus.NewLocalStore("/var/lib/tartarus/layers")
//	oci := erebus.NewOCIStore(local, erebus.OCIConfig{
//	    Registry: "registry.example.com",
//	    Auth:     credentials,
//	})
//
//	// Pull an image and get the local layer path:
//	path, err := oci.Pull(ctx, "my-sandbox:latest")
//
// # Known Technical Debt
//
//   - The [Scanner] integration is a stub; it shells out to `trivy` if
//     available but has no fallback and no policy engine to act on results.
//     A proper advisory-database client and enforcement hook are needed.
//
//   - [S3Store] does not implement multipart upload, limiting single-object
//     uploads to 5 GB. Large root-filesystem snapshots may fail silently.
//
//   - Layer de-duplication relies on sha256 digest matching. There is no
//     garbage collection for orphaned layers; disk usage will grow unboundedly
//     without an external pruning job.
//
//   - The OCI pull path does not support image index (multi-arch) manifests;
//     it always fetches the first manifest in the list. ARM/multi-arch images
//     require explicit digest pinning.
package erebus
