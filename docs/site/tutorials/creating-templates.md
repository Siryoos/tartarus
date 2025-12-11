# Creating Custom Templates

Learn how to create your own sandbox templates for the Tartarus marketplace.

## Template Structure

A template is a YAML file that defines:

- Base image
- Packages to install
- Resource requirements
- Environment variables

## Basic Template

```yaml
apiVersion: v1
kind: Template
metadata:
  name: my-template
  version: "1.0.0"
  author: "Your Name"
  description: "Description of your template"
  category: web
  tags:
    - nodejs
    - api
spec:
  baseImage: node:20-slim
  packages:
    - express
    - typescript
  resources:
    cpu: 2
    memory: 2048
  env:
    NODE_ENV: development
  ports:
    - 3000
```

## Template Fields

### Metadata

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique template identifier |
| `version` | Yes | Semantic version |
| `author` | No | Template author |
| `description` | No | Template description |
| `category` | No | Category (data-science, web, backend, tools) |
| `tags` | No | Searchable tags |

### Spec

| Field | Required | Description |
|-------|----------|-------------|
| `baseImage` | Yes | OCI image to use |
| `packages` | No | Packages to install |
| `resources.cpu` | No | CPU cores (default: 1) |
| `resources.memory` | No | Memory in MB (default: 512) |
| `env` | No | Environment variables |
| `ports` | No | Exposed ports |

## Validation

Validate your template before publishing:

```bash
tartarus template validate ./my-template.yaml
```

## Publishing

Submit your template to the marketplace:

1. Fork the [templates repository](https://github.com/tartarus/templates)
2. Add your template YAML
3. Submit a pull request

## Examples

### Python ML Template

```yaml
apiVersion: v1
kind: Template
metadata:
  name: python-ml
  version: "1.0.0"
spec:
  baseImage: python:3.11-slim
  packages:
    - torch
    - transformers
    - numpy
  resources:
    cpu: 4
    memory: 8192
```

### Go Microservice Template

```yaml
apiVersion: v1
kind: Template
metadata:
  name: go-microservice
  version: "1.0.0"
spec:
  baseImage: golang:1.21-alpine
  resources:
    cpu: 2
    memory: 1024
  env:
    CGO_ENABLED: "0"
```
