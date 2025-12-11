# Getting Started with Tartarus

This guide walks you through installing Tartarus and running your first sandbox.

## Prerequisites

- Linux (required for Firecracker) or macOS (uses gVisor/containerd)
- Docker (for local development)
- Go 1.21+ (for building from source)

## Installation

=== "Quick Install"

    ```bash
    curl -sL https://get.tartarus.io/install.sh | bash
    ```

=== "Homebrew (macOS)"

    ```bash
    brew install tartarus-sandbox/tap/tartarus
    ```

=== "From Source"

    ```bash
    git clone https://github.com/tartarus-sandbox/tartarus
    cd tartarus
    make build
    ```

## Configuration

Create a config file at `~/.tartarus/config.yaml`:

```yaml
current-context: default
contexts:
  default:
    host: http://localhost:8080
    token: your-api-token
```

## Running Your First Sandbox

### 1. Start the API Server

```bash
# Start Olympus API (in development mode)
docker-compose up -d
```

### 2. Create a Sandbox

```bash
# Run a Python data science sandbox
tartarus run --template python-ds --name my-first-sandbox

# Output:
# ✓ Created sandbox my-first-sandbox (id: sbx-abc123)
# ✓ Status: RUNNING
```

### 3. Execute Commands

```bash
# Run a command in the sandbox
tartarus exec my-first-sandbox -- python -c "import numpy; print(numpy.__version__)"

# Attach interactively
tartarus exec -it my-first-sandbox -- /bin/bash
```

### 4. View Logs

```bash
# Stream logs
tartarus logs -f my-first-sandbox
```

### 5. Clean Up

```bash
# Kill the sandbox
tartarus kill my-first-sandbox
```

## Using Templates

Browse available templates:

```bash
tartarus template search python
```

Install a template:

```bash
tartarus template install python-ml
```

## What's Next?

- [Creating Custom Templates](tutorials/creating-templates.md)
- [Kubernetes Deployment](tutorials/kubernetes-deployment.md)
- [API Reference](api/index.md)
