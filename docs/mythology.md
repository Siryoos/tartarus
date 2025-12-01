# The Pantheon: Mythology Primitives

> *"In the beginning there was Chaos, vast and dark. Then appeared Gaea, the deep-breasted earth, and Tartarus, the misty pit in the depths of the earth."* — Hesiod, Theogony

Tartarus is not just a software project; it is a constructed cosmos. Every component is named after a figure or place from Greek mythology, chosen not for aesthetic flair, but to precisely describe its function within the system.

This document serves as the Rosetta Stone for translating between the mythological metaphor and the technical implementation.

## The Three Realms

The system architecture is divided into three distinct planes of existence:

### 1. Olympus (The Control Plane)
*High above the clouds, the gods decree fate.*

Olympus is the API and management layer. It is where "souls" (requests) originate and where their destiny is decided. It is clean, stateless, and authoritative.

- **Olympus (`pkg/olympus`)**: The mountain itself. The central API server and cluster manager.
- **Themis (`pkg/themis`)**: The Titaness of divine law and order. She represents the Policy Engine, defining what is allowed and what is forbidden.
- **Moirai (`pkg/moirai`)**: The Fates (Clotho, Lachesis, Atropos). They are the Scheduler, spinning the thread of execution and deciding on which node a workload will land.
- **Hermes (`pkg/hermes`)**: The messenger of the gods. He handles Telemetry, carrying logs and metrics from the underworld back to the observers.

### 2. Hades (The Cluster Plane)
*The vast, unseen underworld where resources flow.*

Hades is the infrastructure layer—the cluster of physical nodes and the networks that connect them. It is the domain of resource management and routing.

- **Hades (`pkg/hades`)**: The god of the dead and king of the underworld. He represents the Node Registry, maintaining the authoritative view of all physical resources.
- **Cerberus (`pkg/cerberus`)**: The three-headed hound guarding the gates. The Authentication Gateway ensuring only authorized entities can enter.
- **Charon (`pkg/charon`)**: The ferryman. The Load Balancer carrying requests across the river to their destination.

### 3. Tartarus (The Execution Plane)
*The deep abyss where the Titans are imprisoned.*

Tartarus is the runtime environment. It is the deepest part of the system, where code is isolated, executed, and strictly confined.

- **Tartarus (`pkg/tartarus`)**: The pit itself. The Runtime Interface that wraps the Firecracker microVMs.
- **Hecatoncheir (`pkg/hecatoncheir`)**: The "Hundred-Handed Ones" (Briareus, Cottus, Gyes). The Node Agent that guards the host, managing hundreds of concurrent microVMs with its many hands.
- **Typhon (`pkg/typhon`)**: The father of monsters. The Quarantine Pool where dangerous or suspicious workloads are isolated.

## The Five Rivers

The flow of data through the system follows the five rivers of the underworld:

1. **Acheron (`pkg/acheron`)**: *The River of Pain.*
   - **Technical Role**: The Job Queue.
   - **Metaphor**: Requests wait here in agony before being processed. It handles backpressure and prioritization.

2. **Styx (`pkg/styx`)**: *The River of Oaths.*
   - **Technical Role**: The Network Gateway.
   - **Metaphor**: An oath sworn on the Styx is unbreakable. This represents the strict Network Contracts enforced by TAP devices and iptables.

3. **Lethe (`pkg/lethe`)**: *The River of Forgetting.*
   - **Technical Role**: The Ephemeral Filesystem.
   - **Metaphor**: Souls drink from Lethe to forget their past lives. Sandboxes start with a clean slate (overlay FS) and are wiped clean upon termination.

4. **Phlegethon (`pkg/phlegethon`)**: *The River of Fire.*
   - **Technical Role**: The Hot Path Router.
   - **Metaphor**: A river of burning fire. It handles high-performance, compute-intensive workloads ("hot" paths).

5. **Cocytus (`pkg/cocytus`)**: *The River of Wailing.*
   - **Technical Role**: The Error Stream.
   - **Metaphor**: The cries of the damned. All system errors, panics, and failures flow into this stream for analysis.

## The Judges of the Dead

Before a workload can run, it must face judgment (`pkg/judges`).

- **Minos**: The casting vote. Checks **Quotas** and resource availability.
- **Rhadamanthus**: Lord of Elysium. Enforces **Security Policies** and strict isolation.
- **Aeacus**: Guardian of the keys. Handles **Audit** logging and compliance tagging.

## The Primordial Forces

- **Nyx (`pkg/nyx`)**: *Night.* The Snapshot Manager. Before there was anything, there was Night. She creates the base images from which all VMs are born.
- **Erebus (`pkg/erebus`)**: *Darkness.* The Blob Storage. The deep, dark place where images and snapshots are stored at rest.
- **Hypnos (`pkg/hypnos`)**: *Sleep.* The Hibernation Manager. Puts VMs into a suspended state to save resources.
- **Thanatos (`pkg/thanatos`)**: *Death.* The Termination Handler. Ensures a peaceful (graceful) end to a sandbox's life.

## The Furies (Erinyes)

The **Erinyes (`pkg/erinyes`)** are the punishers. They watch running workloads and strike them down if they violate the laws.

- **Alecto ("Unceasing")**: Enforces **Timeouts**. She never stops chasing.
- **Tisiphone ("Avenger")**: Enforces **Resource Limits**. She punishes greed (OOM, CPU hogging).
- **Megaera ("Grudging")**: Enforces **Policy**. She watches for forbidden syscalls or network access.
