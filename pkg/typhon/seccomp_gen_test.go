package typhon

import (
	"testing"
)

func TestSeccompProfileGenerator(t *testing.T) {
	gen := NewSeccompProfileGenerator()

	t.Run("GenerateProfile_Base", func(t *testing.T) {
		profile, err := gen.GenerateProfile("unknown", nil)
		if err != nil {
			t.Fatalf("GenerateProfile failed: %v", err)
		}

		if profile.DefaultAction != "Errno" {
			t.Errorf("Expected DefaultAction Errno, got %s", profile.DefaultAction)
		}

		if len(profile.Syscalls) == 0 {
			t.Fatal("Expected syscalls, got none")
		}

		// Check for some base syscalls
		foundRead := false
		for _, name := range profile.Syscalls[0].Names {
			if name == "read" {
				foundRead = true
				break
			}
		}
		if !foundRead {
			t.Error("Expected 'read' syscall in base profile")
		}
	})

	t.Run("GenerateProfile_ExtraSyscalls", func(t *testing.T) {
		extras := []string{"custom_syscall"}
		profile, err := gen.GenerateProfile("unknown", extras)
		if err != nil {
			t.Fatalf("GenerateProfile failed: %v", err)
		}

		foundCustom := false
		for _, name := range profile.Syscalls[0].Names {
			if name == "custom_syscall" {
				foundCustom = true
				break
			}
		}
		if !foundCustom {
			t.Error("Expected 'custom_syscall' in profile")
		}
	})

	t.Run("GenerateProfileForTemplate", func(t *testing.T) {
		// Just verify it calls the generator
		profile, err := GenerateProfileForTemplate("python-ds")
		if err != nil {
			t.Fatalf("GenerateProfileForTemplate failed: %v", err)
		}
		if profile == nil {
			t.Fatal("Expected profile, got nil")
		}
	})
}
