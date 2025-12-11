# Plugin System

Extend Tartarus with custom judges and furies.

## Overview

Plugins allow you to:

- **Judges**: Control admission and classification
- **Furies**: Enforce custom runtime policies

## Installation

```bash
# List installed plugins
tartarus plugin list

# Install a plugin
tartarus plugin install /path/to/plugin

# Remove a plugin
tartarus plugin remove my-plugin
```

## Plugin Types

| Type | Interface | Purpose |
|------|-----------|---------|
| `judge` | `JudgePlugin` | Pre-admission and post-execution evaluation |
| `fury` | `FuryPlugin` | Runtime policy enforcement |

## Creating Plugins

See tutorials:

- [Writing Custom Judges](../tutorials/custom-judges.md)
- [Writing Custom Furies](../tutorials/custom-furies.md)

## Example Plugins

Pre-built examples in `ecosystem/plugins/`:

| Plugin | Type | Description |
|--------|------|-------------|
| `rate-limit-judge` | judge | Per-tenant request rate limiting |
| `cost-aware-fury` | fury | Cost monitoring and enforcement |

## Plugin Directory

Plugins are installed to `~/.tartarus/plugins/`:

```
~/.tartarus/plugins/
├── rate-limit-judge/
│   ├── manifest.yaml
│   └── rate-limit-judge.so
└── cost-aware-fury/
    ├── manifest.yaml
    └── cost-aware-fury.so
```

## Platform Support

!!! warning "Linux Only"
    Go plugins (`.so` files) only work on Linux due to Go's plugin system limitations.
