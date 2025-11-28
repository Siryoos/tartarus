package tartarus

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"
)

func TestConstructKernelArgs_Networking(t *testing.T) {
	// Replicate the logic from Launch for testing
	construct := func(command []string, args []string, env map[string]string, cfg VMConfig) string {
		kernelArgs := "console=ttyS0 reboot=k panic=1 pci=off"
		if len(command) > 0 {
			var scriptBuilder strings.Builder

			// 0. Configure Network (if provided)
			if cfg.IP.IsValid() {
				bits := cfg.CIDR.Bits()
				scriptBuilder.WriteString(fmt.Sprintf("ip addr add %s/%d dev eth0; ", cfg.IP, bits))
				scriptBuilder.WriteString("ip link set eth0 up; ")
				if cfg.Gateway.IsValid() {
					scriptBuilder.WriteString(fmt.Sprintf("ip route add default via %s; ", cfg.Gateway))
				}
			}

			// 1. Export Environment Variables
			for k, v := range env {
				val := strings.ReplaceAll(v, "'", "'\\''")
				scriptBuilder.WriteString(fmt.Sprintf("export %s='%s'; ", k, val))
			}

			// 2. Build the command
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

	ip, _ := netip.ParseAddr("10.200.0.2")
	gw, _ := netip.ParseAddr("10.200.0.1")
	cidr, _ := netip.ParsePrefix("10.200.0.0/24")

	tests := []struct {
		name    string
		command []string
		args    []string
		env     map[string]string
		cfg     VMConfig
		want    string
	}{
		{
			name:    "With Networking",
			command: []string{"/bin/echo"},
			args:    []string{"hello"},
			env:     nil,
			cfg: VMConfig{
				IP:      ip,
				Gateway: gw,
				CIDR:    cidr,
			},
			// script: ip addr add 10.200.0.2/24 dev eth0; ip link set eth0 up; ip route add default via 10.200.0.1; exec '/bin/echo' 'hello'
			want: `console=ttyS0 reboot=k panic=1 pci=off init=/bin/sh -- -c "ip addr add 10.200.0.2/24 dev eth0; ip link set eth0 up; ip route add default via 10.200.0.1; exec '/bin/echo' 'hello'"`,
		},
		{
			name:    "Without Networking",
			command: []string{"/bin/echo"},
			args:    []string{"hello"},
			env:     nil,
			cfg:     VMConfig{},
			want:    `console=ttyS0 reboot=k panic=1 pci=off init=/bin/sh -- -c "exec '/bin/echo' 'hello'"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := construct(tt.command, tt.args, tt.env, tt.cfg)
			if got != tt.want {
				t.Errorf("construct() = \n%v\nwant \n%v", got, tt.want)
			}
		})
	}
}
