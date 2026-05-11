package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

func TestParseProfile(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		want    domain.Profile
	}{
		{"valid", "phlegethon.large", false, "phlegethon.large"},
		{"valid another", "custom.tier", false, "custom.tier"},
		{"empty", "", true, ""},
		{"no dot", "phlegethonlarge", true, ""},
		{"empty namespace", ".large", true, ""},
		{"empty tier", "phlegethon.", true, ""},
		{"too many dots", "a.b.c", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseProfile(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseProfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseProfile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProfileMethods(t *testing.T) {
	p := domain.Profile("phlegethon.large")
	
	if p.Namespace() != "phlegethon" {
		t.Errorf("Namespace() = %v, want %v", p.Namespace(), "phlegethon")
	}
	
	if p.Tier() != "large" {
		t.Errorf("Tier() = %v, want %v", p.Tier(), "large")
	}
	
	if !p.Valid() {
		t.Errorf("Valid() returned false for a valid profile")
	}
	
	p2 := domain.Profile("invalid")
	if p2.Valid() {
		t.Errorf("Valid() returned true for an invalid profile")
	}
}

func TestProfileJSON(t *testing.T) {
	type TestStruct struct {
		Profile domain.Profile `json:"profile"`
	}
	
	// Test marshal
	s := TestStruct{Profile: "phlegethon.ember"}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if string(data) != `{"profile":"phlegethon.ember"}` {
		t.Errorf("Marshal = %v, want %v", string(data), `{"profile":"phlegethon.ember"}`)
	}
	
	// Test unmarshal
	var s2 TestStruct
	if err := json.Unmarshal([]byte(`{"profile":"phlegethon.flame"}`), &s2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if s2.Profile != "phlegethon.flame" {
		t.Errorf("Unmarshal = %v, want %v", s2.Profile, "phlegethon.flame")
	}
}
