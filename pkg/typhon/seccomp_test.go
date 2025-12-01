package typhon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetQuarantineProfile(t *testing.T) {
	profile, err := GetQuarantineProfile()
	require.NoError(t, err)

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
	profile, err := GetQuarantineStrictProfile()
	require.NoError(t, err)

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
	profile, err := GetDefaultProfile()
	require.NoError(t, err)

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
	profile, err := GetQuarantineProfile()
	require.NoError(t, err)

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
			profile, err := GetProfileByName(tt.name)
			require.NoError(t, err)
			assert.NotNil(t, profile)
			assert.Equal(t, tt.expectedAction, profile.DefaultAction)
		})
	}
}

func TestSeccompProfileGradation(t *testing.T) {
	defaultProfile, err := GetDefaultProfile()
	require.NoError(t, err)
	quarantineProfile, err := GetQuarantineProfile()
	require.NoError(t, err)
	strictProfile, err := GetQuarantineStrictProfile()
	require.NoError(t, err)

	// Strict should have more syscall rules than quarantine, which should have more than default
	assert.Less(t, len(defaultProfile.Syscalls), len(quarantineProfile.Syscalls),
		"Quarantine should have more restrictions than default")
	assert.Less(t, len(quarantineProfile.Syscalls), len(strictProfile.Syscalls),
		"Strict should have more restrictions than quarantine")
}

func TestGetProfileForClass(t *testing.T) {
	// Pre-load expected profiles for comparison
	strict, _ := GetQuarantineStrictProfile()
	quarantine, _ := GetQuarantineProfile()
	def, _ := GetDefaultProfile()

	tests := []struct {
		class           string
		expectedProfile *SeccompProfile
		desc            string
	}{
		{"ember", strict, "Ember should get quarantine-strict profile"},
		{"flame", quarantine, "Flame should get quarantine profile"},
		{"blaze", def, "Blaze should get default profile"},
		{"inferno", def, "Inferno should get default profile"},
		{"unknown", quarantine, "Unknown class should default to quarantine for safety"},
	}

	for _, tt := range tests {
		t.Run(tt.class, func(t *testing.T) {
			profile, err := GetProfileForClass(tt.class)
			require.NoError(t, err)
			// We can't compare pointers directly as they are new structs
			// Compare content by JSON or key fields
			assert.Equal(t, len(tt.expectedProfile.Syscalls), len(profile.Syscalls), tt.desc)
			assert.Equal(t, tt.expectedProfile.DefaultAction, profile.DefaultAction, tt.desc)
		})
	}
}

func TestLoadProfileMissing(t *testing.T) {
	// Try to load a non-existent profile
	profile, err := loadProfile("non_existent_profile")
	assert.Error(t, err)
	assert.Nil(t, profile)
}
