#!/usr/bin/env python3
import os
import glob
import subprocess
import re

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUTPUT_FILE = os.path.join(PROJECT_ROOT, "llms.txt")

def generate_llms_txt():
    with open(OUTPUT_FILE, "w") as f:
        f.write("# Tartarus: Hyperscale MicroVM Orchestrator\n\n")
        f.write("Tartarus is a hyperscale microVM sandbox orchestrator designed to execute untrusted code with the speed of containers and the security of virtual machines.\n\n")
        
        f.write("## Mythology Dictionary (Package Map)\n\n")
        pkg_dir = os.path.join(PROJECT_ROOT, "pkg")
        packages = sorted([d for d in os.listdir(pkg_dir) if os.path.isdir(os.path.join(pkg_dir, d))])
        
        for pkg in packages:
            doc_go = os.path.join(pkg_dir, pkg, "doc.go")
            if os.path.exists(doc_go):
                with open(doc_go, "r") as doc_f:
                    lines = doc_f.readlines()
                    doc_lines = [l.strip().strip("// ") for l in lines if l.startswith("//")]
                    if doc_lines:
                        f.write(f"- **`{pkg}`**: {' '.join(doc_lines)}\n")

        f.write("\n## Core Interfaces\n\n")
        # Extract interfaces using go doc or regex
        # We'll use a regex to find exported interfaces in all .go files in pkg/
        interfaces = []
        for root, dirs, files in os.walk(pkg_dir):
            for file in files:
                if file.endswith(".go") and not file.endswith("_test.go"):
                    with open(os.path.join(root, file), "r") as src_f:
                        content = src_f.read()
                        # Find interface definitions
                        matches = re.finditer(r"^type\s+([A-Z]\w*)\s+interface\s+\{([^}]+)\}", content, re.MULTILINE)
                        for match in matches:
                            interface_name = match.group(1)
                            body = match.group(2).strip()
                            pkg_name = os.path.basename(root)
                            interfaces.append((pkg_name, interface_name, body))
        
        # Sort interfaces to ensure deterministic output
        interfaces.sort(key=lambda x: (x[0], x[1]))
        
        for pkg_name, interface_name, body in interfaces:
            f.write(f"### `{pkg_name}.{interface_name}`\n")
            f.write("```go\n")
            f.write(f"type {interface_name} interface {{\n")
            for line in body.split("\n"):
                f.write(f"    {line.strip()}\n")
            f.write("}\n```\n\n")

        f.write("## Build Tags & Stub Pattern\n\n")
        f.write("The repository uses a pervasive `*_stub.go` pattern (e.g., `firecracker_runtime_stub.go`). ")
        f.write("These files use Go build tags (like `//go:build !firecracker`) to provide fallback implementations when specific dependencies are missing. ")
        f.write("When modifying components with stubs, ensure both the main implementation and the stub are updated, and respect the build tags to avoid breaking compilation.\n\n")

        f.write("## CRD Type Schemas\n\n")
        # Extract CRD Types from pkg/kubernetes/apis/tartarus/v1alpha1
        crd_dir = os.path.join(PROJECT_ROOT, "pkg", "kubernetes", "apis", "tartarus", "v1alpha1")
        if os.path.exists(crd_dir):
            crd_structs = []
            for file in ["sandboxjob_types.go", "sandboxtemplate_types.go", "tenantnetworkpolicy_types.go"]:
                file_path = os.path.join(crd_dir, file)
                if os.path.exists(file_path):
                    with open(file_path, "r") as src_f:
                        content = src_f.read()
                        matches = re.finditer(r"^type\s+([A-Z]\w*(?:Spec|Status|))\s+struct\s+\{([^}]+)\}", content, re.MULTILINE)
                        for match in matches:
                            struct_name = match.group(1)
                            body = match.group(2).strip()
                            crd_structs.append((struct_name, body))
            
            crd_structs.sort(key=lambda x: x[0])
            for struct_name, body in crd_structs:
                f.write(f"### `{struct_name}`\n")
                f.write("```go\n")
                f.write(f"type {struct_name} struct {{\n")
                for line in body.split("\n"):
                    f.write(f"    {line.strip()}\n")
                f.write("}\n```\n\n")

        f.write("## Plugin Manifest Format\n\n")
        f.write("Plugins are defined using a manifest format (typically `manifest.yaml`). The extensibility framework uses adapters like `FuryAdapter` and `JudgeAdapter` to load custom components.\n")

    print(f"Generated {OUTPUT_FILE}")

if __name__ == "__main__":
    generate_llms_txt()
