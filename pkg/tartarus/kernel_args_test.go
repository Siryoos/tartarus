package tartarus

import (
	"fmt"
	"strings"
	"testing"
)

func TestConstructKernelArgs(t *testing.T) {
	// Replicate the logic from Launch for testing
	construct := func(command []string, args []string, env map[string]string) string {
		kernelArgs := "console=ttyS0 reboot=k panic=1 pci=off"
		if len(command) > 0 {
			var scriptBuilder strings.Builder

			// Deterministic order for test? Map iteration is random.
			// We can't easily test exact string match with map iteration unless we sort.
			// But for this test let's just use one env var.
			for k, v := range env {
				val := strings.ReplaceAll(v, "'", "'\\''")
				scriptBuilder.WriteString(fmt.Sprintf("export %s='%s'; ", k, val))
			}

			fullCmd := append(command, args...)
			scriptBuilder.WriteString("exec")
			for _, part := range fullCmd {
				arg := strings.ReplaceAll(part, "'", "'\\''")
				scriptBuilder.WriteString(fmt.Sprintf(" '%s'", arg))
			}

			script := scriptBuilder.String()
			scriptEscaped := strings.ReplaceAll(script, "\"", "\\\"")

			kernelArgs = fmt.Sprintf("%s init=/bin/sh -- -c \"%s\"", kernelArgs, scriptEscaped)
		}
		return kernelArgs
	}

	tests := []struct {
		name    string
		command []string
		args    []string
		env     map[string]string
		want    string
	}{
		{
			name:    "Simple command",
			command: []string{"/bin/echo"},
			args:    []string{"hello"},
			env:     nil,
			want:    `console=ttyS0 reboot=k panic=1 pci=off init=/bin/sh -- -c "exec '/bin/echo' 'hello'"`,
		},
		{
			name:    "Command with spaces and quotes",
			command: []string{"/bin/sh"},
			args:    []string{"-c", "echo 'hello world'"},
			env:     nil,
			// script: exec '/bin/sh' '-c' 'echo '\''hello world'\'''
			// escaped: exec '/bin/sh' '-c' 'echo '\''hello world'\''' (no double quotes to escape)
			want: `console=ttyS0 reboot=k panic=1 pci=off init=/bin/sh -- -c "exec '/bin/sh' '-c' 'echo '\''hello world'\'''"`,
		},
		{
			name:    "Env vars",
			command: []string{"/app/run"},
			args:    nil,
			env:     map[string]string{"FOO": "BAR"},
			// script: export FOO='BAR'; exec '/app/run'
			want: `console=ttyS0 reboot=k panic=1 pci=off init=/bin/sh -- -c "export FOO='BAR'; exec '/app/run'"`,
		},
		{
			name:    "Env with quotes",
			command: []string{"/app/run"},
			args:    nil,
			env:     map[string]string{"MSG": "It's me"},
			// script: export MSG='It'\''s me'; exec '/app/run'
			want: `console=ttyS0 reboot=k panic=1 pci=off init=/bin/sh -- -c "export MSG='It'\''s me'; exec '/app/run'"`,
		},
		{
			name:    "Env with double quotes",
			command: []string{"/app/run"},
			args:    nil,
			env:     map[string]string{"JSON": `{"a":1}`},
			// script: export JSON='{"a":1}'; exec '/app/run'
			// escaped: export JSON='{\"a\":1}'; exec '/app/run'
			want: `console=ttyS0 reboot=k panic=1 pci=off init=/bin/sh -- -c "export JSON='{\"a\":1}'; exec '/app/run'"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := construct(tt.command, tt.args, tt.env)
			if got != tt.want {
				t.Errorf("construct() = %v, want %v", got, tt.want)
			}
		})
	}
}
