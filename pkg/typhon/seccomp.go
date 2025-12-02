package typhon

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed profiles/*.json
var profileFS embed.FS

// Seccomp profile constants
const (
	SeccompDefault          = "default"
	SeccompQuarantine       = "quarantine"        // Maps to flame
	SeccompQuarantineStrict = "quarantine-strict" // Maps to ember
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

var (
	profileCache sync.Map
)

// loadProfile loads a profile from the embedded FS
func loadProfile(name string) (*SeccompProfile, error) {
	if val, ok := profileCache.Load(name); ok {
		return val.(*SeccompProfile), nil
	}

	data, err := profileFS.ReadFile("profiles/" + name + ".json")
	if err != nil {
		return nil, fmt.Errorf("failed to read profile %s: %w", name, err)
	}

	var profile SeccompProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to unmarshal profile %s: %w", name, err)
	}

	profileCache.Store(name, &profile)
	return &profile, nil
}

// GetQuarantineProfile returns the standard quarantine seccomp profile (flame)
func GetQuarantineProfile() (*SeccompProfile, error) {
	return loadProfile("flame")
}

// GetQuarantineStrictProfile returns an even more restrictive profile (ember)
func GetQuarantineStrictProfile() (*SeccompProfile, error) {
	return loadProfile("ember")
}

// GetDefaultProfile returns the default (permissive) seccomp profile
func GetDefaultProfile() (*SeccompProfile, error) {
	return loadProfile("default")
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
func GetProfileByName(name string) (*SeccompProfile, error) {
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
func GetProfileForClass(class string) (*SeccompProfile, error) {
	switch class {
	case "ember":
		// Ember (Cold) gets strictest isolation - quarantine-strict profile
		return GetQuarantineStrictProfile()
	case "flame":
		// Flame (Warm) gets quarantine profile
		return GetQuarantineProfile()
	case "blaze":
		// Blaze (Hot) gets moderate restrictions - default profile
		return GetDefaultProfile()
	case "inferno":
		// Inferno (highest resource, GPU-capable) gets permissive default
		return GetDefaultProfile()
	default:
		// Unknown classes default to quarantine for safety
		return GetQuarantineProfile()
	}
}

// GenerateProfileForTemplate generates a seccomp profile for a specific template
func GenerateProfileForTemplate(templateID string) (*SeccompProfile, error) {
	gen := NewSeccompProfileGenerator()
	return gen.GenerateProfile(templateID, nil)
}
