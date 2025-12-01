# Tartarus: The Underworld Operating System

> *"Below even Hades lies Tartarus—as far beneath the earth as the sky is above it."* — Hesiod

**Tartarus** is a hyperscale microVM sandbox orchestrator designed to execute untrusted code with the speed of containers and the security of virtual machines. It is the "Underworld Operating System," a cosmic machine that judges, isolates, and executes millions of "souls" (workloads) every day.

Built on Firecracker and heavily inspired by Greek mythology, Tartarus provides a strict, policy-driven environment where every process is judged, every resource is accounted for, and every error is mourned.

## ⚡ Quick Start

To descend into the underworld and start your first sandbox:

### Prerequisites
- Linux (kernel 5.10+) with KVM enabled
- Docker & Docker Compose
- Go 1.21+

### Running the Stack

1. **Summon the Deities** (Start Control Plane & Agent):
   ```bash
   docker-compose up -d
   ```

2. **Cast a Soul into the Pit** (Run a Sandbox):
   ```bash
   # Build the CLI
   go build -o tartarus ./cmd/tartarus

   # Launch a Python sandbox
   ./tartarus run --template python-ds -- python -c "print('Hello from the Underworld')"
   ```

3. **Observe the Realm**:
   ```bash
   ./tartarus ps
   ```

## 🏛️ Architecture Overview

The system is organized into three realms, mirroring the Greek cosmos:

### 1. Olympus (Control Plane)
The seat of power where policies are defined and judgments are made.
- **Olympus**: The API Gateway and central manager.
- **Themis**: The Policy Engine defining what is allowed.
- **Moirai**: The Fates (Scheduler) deciding where workloads run.
- **Hermes**: The Messenger delivering telemetry and logs.

### 2. Hades (Cluster Resources)
The infrastructure layer managing the flow of resources and data.
- **Hades**: The Node Registry and cluster view.
- **Acheron**: The River of Pain (Job Queue) where requests wait.
- **Styx**: The River of Oaths (Network Gateway) enforcing contracts.
- **Cocytus**: The River of Wailing (Error Stream) collecting failures.

### 3. Tartarus (Sandbox Execution)
The deep pit where code is actually executed.
- **Hecatoncheir**: The Hundred-Handed One (Node Agent) guarding the host.
- **Tartarus**: The Runtime Interface controlling Firecracker microVMs.
- **Nyx**: The Night (Snapshot Manager) enabling sub-second starts.
- **Lethe**: The River of Forgetting (Ephemeral Filesystem) ensuring clean slates.

## 🗺️ Documentation Map

- **[The Prophecy (Roadmap)](./ROADMAP.md)**: The past, present, and future of the project.
- **[The Pantheon (Mythology)](./docs/mythology.md)**: A guide to the mythological naming conventions and their technical meanings.
- **[The Blueprint (Architecture)](./docs/architecture.md)**: A deep technical dive into the system's design and data flows.

## ✍️ Credits

**Architect & Sole Developer**: siryoos

*Tartarus is a single-developer feat, crafted to push the boundaries of isolation technology.*
