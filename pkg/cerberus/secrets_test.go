package cerberus

import (
	"context"
	"os"
	"testing"
)

func TestEnvSecretProvider(t *testing.T) {
	os.Setenv("TEST_SECRET", "secret-value")
	os.Setenv("CERBERUS_KEY_testkey", "signing-key-value")
	defer os.Unsetenv("TEST_SECRET")
	defer os.Unsetenv("CERBERUS_KEY_testkey")

	p := NewEnvSecretProvider()
	ctx := context.Background()

	tests := []struct {
		name    string
		ref     string
		want    string
		wantErr bool
	}{
		{"ValidEnv", "env:TEST_SECRET", "secret-value", false},
		{"ValidKey", "key:testkey", "signing-key-value", false},
		{"MissingEnv", "env:MISSING", "", true},
		{"MissingKey", "key:missing", "", true},
		{"InvalidFormat", "foo:bar", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Resolve(ctx, tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Resolve() got = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockProvider struct {
	data map[string]string
}

func (m *mockProvider) Resolve(ctx context.Context, ref string) (string, error) {
	if val, ok := m.data[ref]; ok {
		return val, nil
	}
	return "", os.ErrNotExist
}

func TestCompositeSecretProvider(t *testing.T) {
	p1 := &mockProvider{data: map[string]string{"p1:k1": "v1"}}
	p2 := &mockProvider{data: map[string]string{"p2:k2": "v2"}}

	c := NewCompositeSecretProvider(p1, p2)
	ctx := context.Background()

	tests := []struct {
		name    string
		ref     string
		want    string
		wantErr bool
	}{
		{"Provider1", "p1:k1", "v1", false},
		{"Provider2", "p2:k2", "v2", false},
		{"Missing", "p3:k3", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.Resolve(ctx, tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Resolve() got = %v, want %v", got, tt.want)
			}
		})
	}
}
