package typhon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetQuarantineProfile(t *testing.T) {
	profile := GetQuarantineProfile()

	assert.Equal(t, "SCMP_ACT_ALLOW", profile.DefaultAction)
	assert.NotEmpty(t, profile.Syscalls)

	// Verify network syscalls are blocked
	hasNetworkBlock := false
	for _, sc := range profile.Syscalls {
		for _, name := range sc.Names {
			if name == "socket" || name == "connect" {
				hasNetworkBlock = true
				assert.Equal(t, "SCMP_ACT_ERRNO", sc.Action)
			}
		}
	}
	assert.True(t, hasNetworkBlock, "Should block network syscalls")
}

func TestGetQuarantineStrictProfile(t *testing.T) {
	profile := GetQuarantineStrictProfile()

	assert.Equal(t, "SCMP_ACT_ALLOW", profile.DefaultAction)
	assert.NotEmpty(t, profile.Syscalls)

	// Verify additional restrictions beyond base quarantine
	hasChmodBlock := false
	hasIPCBlock := false
	hasCapBlock := false

	for _, sc := range profile.Syscalls {
		for _, name := range sc.Names {
			if name == "chmod" || name == "fchmod" {
				hasChmodBlock = true
				assert.Equal(t, "SCMP_ACT_ERRNO", sc.Action)
			}
			if name == "msgget" || name == "semget" {
				hasIPCBlock = true
				assert.Equal(t, "SCMP_ACT_ERRNO", sc.Action)
			}
			if name == "capset" || name == "setuid" {
				hasCapBlock = true
				assert.Equal(t, "SCMP_ACT_ERRNO", sc.Action)
			}
		}
	}

	assert.True(t, hasChmodBlock, "Strict profile should block chmod")
	assert.True(t, hasIPCBlock, "Strict profile should block IPC")
	assert.True(t, hasCapBlock, "Strict profile should block capability changes")
}

func TestGetDefaultProfile(t *testing.T) {
	profile := GetDefaultProfile()

	assert.Equal(t, "SCMP_ACT_ALLOW", profile.DefaultAction)
	assert.NotEmpty(t, profile.Syscalls)

	// Default profile should be permissive, only blocking the most dangerous
	dangerousBlocked := false
	for _, sc := range profile.Syscalls {
		for _, name := range sc.Names {
			if name == "reboot" || name == "init_module" {
				dangerousBlocked = true
				assert.Equal(t, "SCMP_ACT_ERRNO", sc.Action)
			}
		}
	}
	assert.True(t, dangerousBlocked, "Should block dangerous syscalls")
}

func TestSeccompProfileToJSON(t *testing.T) {
	profile := GetQuarantineProfile()

	jsonStr, err := profile.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, jsonStr)
	assert.Contains(t, jsonStr, "default_action")
	assert.Contains(t, jsonStr, "syscalls")
	assert.Contains(t, jsonStr, "SCMP_ACT_ALLOW")
}

func TestGetProfileByName(t *testing.T) {
	tests := []struct {
		name           string
		expectedAction string
	}{
		{SeccompDefault, "SCMP_ACT_ALLOW"},
		{SeccompQuarantine, "SCMP_ACT_ALLOW"},
		{SeccompQuarantineStrict, "SCMP_ACT_ALLOW"},
		{"unknown", "SCMP_ACT_ALLOW"}, // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := GetProfileByName(tt.name)
			assert.NotNil(t, profile)
			assert.Equal(t, tt.expectedAction, profile.DefaultAction)
		})
	}
}

func TestSeccompProfileGradation(t *testing.T) {
	defaultProfile := GetDefaultProfile()
	quarantineProfile := GetQuarantineProfile()
	strictProfile := GetQuarantineStrictProfile()

	// Strict should have more syscall rules than quarantine, which should have more than default
	assert.Less(t, len(defaultProfile.Syscalls), len(quarantineProfile.Syscalls),
		"Quarantine should have more restrictions than default")
	assert.Less(t, len(quarantineProfile.Syscalls), len(strictProfile.Syscalls),
		"Strict should have more restrictions than quarantine")
}
