# Writing Custom Judges

Judges evaluate sandbox requests before execution (PreJudge) and after completion (PostJudge).

## Judge Interfaces

```go
// PreJudge runs before scheduling
type PreJudge interface {
    PreAdmit(ctx context.Context, req *domain.SandboxRequest) (Verdict, error)
}

// PostJudge runs after completion
type PostJudge interface {
    PostHoc(ctx context.Context, run *domain.SandboxRun) (*Classification, error)
}
```

## Verdicts

| Verdict | Description |
|---------|-------------|
| `VerdictAccept` | Allow the request |
| `VerdictReject` | Deny the request |
| `VerdictQuarantine` | Run in isolated mode |

## Creating a Plugin Judge

### 1. Create Plugin Directory

```bash
mkdir -p ~/.tartarus/plugins/my-judge
```

### 2. Create Manifest

```yaml
# manifest.yaml
apiVersion: v1
kind: TartarusPlugin
metadata:
  name: my-judge
  version: 1.0.0
  author: Your Name
  description: Custom admission judge
spec:
  type: judge
  entryPoint: my-judge.so
  config:
    maxMemoryMB: 4096
```

### 3. Implement the Plugin

```go
package main

import (
    "context"
    "github.com/tartarus-sandbox/tartarus/pkg/domain"
    "github.com/tartarus-sandbox/tartarus/pkg/plugins"
)

type MyJudge struct {
    maxMemory int
}

func (j *MyJudge) Name() string           { return "my-judge" }
func (j *MyJudge) Version() string        { return "1.0.0" }
func (j *MyJudge) Type() plugins.PluginType { return plugins.PluginTypeJudge }

func (j *MyJudge) Init(config map[string]any) error {
    if v, ok := config["maxMemoryMB"].(int); ok {
        j.maxMemory = v
    }
    return nil
}

func (j *MyJudge) Close() error { return nil }

func (j *MyJudge) PreAdmit(ctx context.Context, req *domain.SandboxRequest) (plugins.Verdict, error) {
    if req.Resources.Mem > domain.Megabytes(j.maxMemory) {
        return plugins.VerdictReject, nil
    }
    return plugins.VerdictAccept, nil
}

func (j *MyJudge) PostHoc(ctx context.Context, run *domain.SandboxRun) (*plugins.Classification, error) {
    return nil, nil
}

var TartarusPlugin plugins.JudgePlugin = &MyJudge{}
```

### 4. Build the Plugin

```bash
go build -buildmode=plugin -o my-judge.so main.go
```

!!! warning "Platform Requirement"
    Go plugins only work on Linux with matching Go versions.

### 5. Install

```bash
tartarus plugin install /path/to/my-judge
```

## Built-in Judges

| Judge | Description |
|-------|-------------|
| `ResourceJudge` | Validates resource requests against policy |
| `NetworkJudge` | Validates network policies |
| `AeacusJudge` | Audit and compliance tagging |
