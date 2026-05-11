// Package lethe implements the ephemeral filesystem manager — the River of
// Forgetfulness that ensures each sandbox starts with a clean slate.
//
// Named after the river whose waters caused the souls to forget their past
// lives, Lethe provides copy-on-write (CoW) filesystem overlays so that
// every sandbox run begins from a pristine base image without modifying the
// shared layer cache.
//
// # Core Types
//
//   - [Pool]: Manages a pool of pre-provisioned overlay directories. Callers
//     check out an overlay before launching a sandbox and return it after.
//
//   - [FileOverlay]: A single CoW overlay using Linux overlayfs (or a copy
//     fallback on non-Linux systems). Wraps an upper dir, a work dir, and
//     a merged mount point.
//
// # Basic Usage
//
//	pool := lethe.NewPool("/var/lib/tartarus/overlays", 10)
//
//	// Before launch:
//	overlay, err := pool.Checkout(ctx, baseRootFSPath)
//	defer pool.Return(overlay)
//
//	// overlay.MergedPath is the root filesystem to pass to the runtime.
//
// # Known Technical Debt
//
//   - [FileOverlay] uses overlayfs mount syscalls which require CAP_SYS_ADMIN
//     or user-namespace support. In environments that restrict these (e.g.,
//     some managed K8s nodes), a full copy fallback is used which is
//     significantly slower (seconds vs. milliseconds for large images).
//
//   - Pool pre-warming is not yet implemented. Overlays are created on demand,
//     adding latency to the first sandbox launch after a quiet period.
//     Persephone can predict demand, but the pre-warm hook into Lethe is
//     not yet wired.
//
//   - There is no maximum pool size enforcement at the OS level; pool
//     exhaustion returns an error but does not block or queue the caller.
//     Under heavy concurrent load, callers may be rejected even if overlays
//     would become available momentarily.
//
//   - Cleanup of orphaned overlay mounts (e.g., after a crash) is not
//     automated. A startup reconciliation scan is needed.
package lethe
