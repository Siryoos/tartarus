# Tartarus v1.0 Tasks

Context: The architecture is present but key subsystems are stubbed or missing, preventing the “Docker image to microVM” promise from working. The items below capture what must be built to reach a functional v1.0 as described in `ROADMAP.md` and the Vision docs.

- **Erebus OCI builder (rootfs image)** [COMPLETED]
  - Context: `pkg/erebus/oci.go:BuildRootFS` currently writes a dummy file. No bootable disk is produced, so Firecracker cannot mount `/dev/vda`.
  - DoD: Extracted OCI rootfs is packaged into a bootable ext4 image (e.g., via `genext2fs`/`virt-make-fs` or equivalent Go lib); image produced at the expected `dstFile`; builder returns error on tool failure; covered by an integration test that boots a minimal VM with the generated image.

- **Nyx warmup/snapshot flow** [COMPLETED]
  - Context: `Nyx.Prepare` is unimplemented. Hecatoncheir calls `Nyx.GetSnapshot`, but no VM is actually started/paused/snapshotted to enable sub-second cold starts.
  - DoD: `Prepare` orchestrates runtime `Start -> Pause -> CreateSnapshot` for a template workload; snapshot metadata is stored/retrievable; a smoke test proves `GetSnapshot` returns a usable artifact for a new launch.

- **Agent dependencies in image** [COMPLETED]
  - Context: Agent runtime requires system binaries: `firecracker`, `iptables`/`iproute2`, and the new rootfs builder dependency (`genext2fs` or chosen tool). `Dockerfile.agent` does not install them.
  - DoD: `Dockerfile.agent` installs and pins required binaries; image build succeeds from clean checkout; agent starts without missing-binary errors in containerized deploy.

- **Olympus startup reconciliation**
  - Context: `pkg/olympus/manager.go` does not rebuild state on restart. If Olympus crashes while agents run sandboxes, state is lost.
  - DoD: `Reconcile()` queries Hades for nodes, asks each agent for running sandboxes, and repopulates in-memory state on startup; safe to call repeatedly; unit test or controlled integration test demonstrates state is restored after a manager restart.

- **Typhon seccomp profiles present**
  - Context: `firecracker_runtime` loads profiles via `typhon.GetProfileForClass(class)` but may lack concrete JSON definitions (ember/flame).
  - DoD: Seccomp JSON for required classes lives in `pkg/typhon`; `Launch` succeeds when profiles are present; failing lookup yields clear errors; minimal test validates profiles load.
