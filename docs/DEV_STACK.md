# Tartarus Development Stack

This guide explains how to run the Tartarus stack locally for development.

## Prerequisites

1.  **Docker & Docker Compose**: Ensure you have Docker installed and running.
2.  **KVM Support**: The host machine must support KVM (Kernel-based Virtual Machine) and expose `/dev/kvm`.
    *   **Linux**: Native support. Ensure your user has permissions (usually `kvm` group).
    *   **macOS/Windows**: Requires nested virtualization if running inside a VM, or bare metal Linux. *Note: Firecracker on Docker for Mac is experimental and may require specific configurations or a Linux VM.*

## Quick Start

1.  **Run the Setup Script**:
    This script downloads a sample kernel and rootfs to `./data/firecracker` and checks for KVM access.
    ```bash
    ./dev-setup.sh
    ```

2.  **Start the Stack**:
    Boots Redis, MinIO, Olympus API, and the Hecatoncheir Agent.
    ```bash
    docker-compose up --build
    ```

## Architecture

The dev stack consists of:

*   **Redis**: Backing store for Hades (Registry) and Acheron (Queue).
*   **MinIO**: S3-compatible object storage for snapshots.
*   **Olympus API**: The control plane API.
*   **Hecatoncheir Agent**: The node agent that runs Firecracker microVMs.

## Troubleshooting

*   **Agent fails to start**: Check if `/dev/kvm` is accessible. You might need `sudo chmod 666 /dev/kvm` on the host.
*   **Networking issues**: The agent uses TAP devices. The Docker container runs in `privileged` mode to allow this.
