package typhon

import (
	"encoding/json"
)

// Seccomp profile constants
const (
	SeccompDefault          = "default"
	SeccompQuarantine       = "quarantine"
	SeccompQuarantineStrict = "quarantine-strict"
)

// SeccompProfile represents a seccomp configuration for Firecracker
type SeccompProfile struct {
	DefaultAction string    `json:"default_action"`
	Syscalls      []Syscall `json:"syscalls"`
}

// Syscall represents a syscall rule
type Syscall struct {
	Names  []string `json:"names"`
	Action string   `json:"action"`
}

// GetQuarantineProfile returns the standard quarantine seccomp profile
// Blocks: network syscalls, ptrace, kernel modules, privileged operations
func GetQuarantineProfile() *SeccompProfile {
	return &SeccompProfile{
		DefaultAction: "SCMP_ACT_ALLOW",
		Syscalls: []Syscall{
			// Block network syscalls
			{
				Names: []string{
					"socket",
					"bind",
					"connect",
					"listen",
					"accept",
					"accept4",
					"sendto",
					"sendmsg",
					"recvfrom",
					"recvmsg",
				},
				Action: "SCMP_ACT_ERRNO",
			},
			// Block ptrace and process inspection
			{
				Names: []string{
					"ptrace",
					"process_vm_readv",
					"process_vm_writev",
				},
				Action: "SCMP_ACT_ERRNO",
			},
			// Block kernel module operations
			{
				Names: []string{
					"init_module",
					"finit_module",
					"delete_module",
				},
				Action: "SCMP_ACT_ERRNO",
			},
			// Block privileged operations
			{
				Names: []string{
					"reboot",
					"swapon",
					"swapoff",
					"mount",
					"umount",
					"umount2",
					"pivot_root",
					"chroot",
				},
				Action: "SCMP_ACT_ERRNO",
			},
		},
	}
}

// GetQuarantineStrictProfile returns an even more restrictive profile
// Adds: restricted file operations, IPC restrictions, capability restrictions
func GetQuarantineStrictProfile() *SeccompProfile {
	base := GetQuarantineProfile()

	// Add additional restrictions
	base.Syscalls = append(base.Syscalls, []Syscall{
		// Block certain file operations
		{
			Names: []string{
				"chmod",
				"fchmod",
				"fchmodat",
				"chown",
				"fchown",
				"lchown",
				"fchownat",
			},
			Action: "SCMP_ACT_ERRNO",
		},
		// Block IPC mechanisms
		{
			Names: []string{
				"msgget",
				"msgsnd",
				"msgrcv",
				"msgctl",
				"semget",
				"semop",
				"semctl",
				"shmget",
				"shmat",
				"shmdt",
				"shmctl",
			},
			Action: "SCMP_ACT_ERRNO",
		},
		// Block capability changes
		{
			Names: []string{
				"capset",
				"setuid",
				"setgid",
				"setreuid",
				"setregid",
				"setresuid",
				"setresgid",
				"setfsuid",
				"setfsgid",
			},
			Action: "SCMP_ACT_ERRNO",
		},
	}...)

	return base
}

// GetDefaultProfile returns the default (permissive) seccomp profile
func GetDefaultProfile() *SeccompProfile {
	return &SeccompProfile{
		DefaultAction: "SCMP_ACT_ALLOW",
		Syscalls: []Syscall{
			// Only block the most dangerous syscalls
			{
				Names: []string{
					"reboot",
					"init_module",
					"finit_module",
					"delete_module",
				},
				Action: "SCMP_ACT_ERRNO",
			},
		},
	}
}

// ToJSON serializes the seccomp profile to JSON for Firecracker
func (p *SeccompProfile) ToJSON() (string, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetProfileByName returns a seccomp profile by name
func GetProfileByName(name string) *SeccompProfile {
	switch name {
	case SeccompQuarantine:
		return GetQuarantineProfile()
	case SeccompQuarantineStrict:
		return GetQuarantineStrictProfile()
	case SeccompDefault:
		return GetDefaultProfile()
	default:
		return GetDefaultProfile()
	}
}

// GetProfileForClass returns the appropriate seccomp profile for a resource class
// Graduated approach:
// - ember (cold, <10s, <512MB): Strictest isolation with quarantine-strict
// - flame (warm, <5min, <4GB): Quarantine profile blocking network/privileged ops
// - blaze (hot, <30min, <16GB): Moderate restrictions with default profile
// - inferno (unlimited, GPU): Permissive default (needs networking, GPU access)
func GetProfileForClass(class string) *SeccompProfile {
	switch class {
	case "ember":
		// Ember (Cold) gets strictest isolation - quarantine-strict profile
		// Short-lived tasks don't need file ownership changes, IPC, or capabilities
		return GetQuarantineStrictProfile()
	case "flame":
		// Flame (Warm) gets quarantine profile
		// Block network, ptrace, kernel modules, privileged ops
		return GetQuarantineProfile()
	case "blaze":
		// Blaze (Hot) gets moderate restrictions - default profile
		// Allow most operations but block dangerous syscalls
		return GetDefaultProfile()
	case "inferno":
		// Inferno (highest resource, GPU-capable) gets permissive default
		// Needs maximum flexibility for long-running, high-resource workloads
		return GetDefaultProfile()
	default:
		// Unknown classes default to quarantine for safety
		return GetQuarantineProfile()
	}
}
