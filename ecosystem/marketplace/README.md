# Tartarus Template Marketplace

Welcome to the Tartarus Template Marketplace! Find community-contributed templates for various use cases.

## Quick Start

```bash
# Search templates
tartarus template search python

# Install a template
tartarus template install python-ds

# View available templates
tartarus template list
```

## Available Templates

| Name | Category | Description | Version |
|------|----------|-------------|---------|
| python-ds | data-science | Python Data Science with NumPy, Pandas, Scikit-learn | 1.0.0 |
| python-ml | data-science | Python ML with PyTorch and TensorFlow | 1.1.0 |
| node-api | web | Node.js API with Express and TypeScript | 1.2.0 |
| go-microservice | backend | Go microservice with gRPC and Prometheus | 1.0.0 |
| rust-cli | tools | Rust CLI with clap and tokio | 0.9.0 |
| static-frontend | web | Static frontend with Vite, React, TailwindCSS | 2.0.0 |

## Template Structure

Each template consists of:
- `manifest.yaml` - Template metadata and specification
- Supporting files (Dockerfiles, configs, etc.)

### Template Manifest Format

```yaml
apiVersion: v1
kind: Template
metadata:
  name: my-template
  version: "1.0.0"
  author: "Your Name"
  description: "Template description"
  category: web
  tags:
    - tag1
    - tag2
spec:
  baseImage: ubuntu:22.04
  packages:
    - package1
    - package2
  resources:
    cpu: 2
    memory: 4096
```

## Contributing

### Submit a Template

1. Fork the [tartarus-templates](https://github.com/tartarus/templates) repository
2. Create a directory for your template
3. Add `manifest.yaml` with required fields
4. Test with `tartarus template validate ./my-template`
5. Submit a PR

### Verification

Templates marked as **verified** (✓) have been reviewed by the Tartarus team.

### Guidelines

- Use semantic versioning
- Include clear descriptions
- Tag appropriately for discoverability
- Test on both arm64 and amd64

## Categories

| Icon | Category | Description |
|------|----------|-------------|
| 📊 | data-science | Data science and ML environments |
| 🌐 | web | Web applications and APIs |
| ⚙️ | backend | Backend services and microservices |
| 🔧 | tools | CLI tools and utilities |
