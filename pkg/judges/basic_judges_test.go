package judges

import (
	"context"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

func TestNetworkJudge_PreAdmit(t *testing.T) {
	logger := hermes.NewSlogAdapter()

	tests := []struct {
		name            string
		allowedNetworks []string
		req             *domain.SandboxRequest
		want            Verdict
	}{
		{
			name:            "Allow default no-net",
			allowedNetworks: []string{},
			req: &domain.SandboxRequest{
				NetworkRef: domain.NetworkPolicyRef{ID: "no-net"},
			},
			want: VerdictAccept,
		},
		{
			name:            "Allow default lockdown",
			allowedNetworks: []string{},
			req: &domain.SandboxRequest{
				NetworkRef: domain.NetworkPolicyRef{ID: "lockdown"},
			},
			want: VerdictAccept,
		},
		{
			name:            "Reject unknown network when list empty",
			allowedNetworks: []string{},
			req: &domain.SandboxRequest{
				NetworkRef: domain.NetworkPolicyRef{ID: "custom-net"},
			},
			want: VerdictReject,
		},
		{
			name:            "Allow configured network",
			allowedNetworks: []string{"custom-net", "other-net"},
			req: &domain.SandboxRequest{
				NetworkRef: domain.NetworkPolicyRef{ID: "custom-net"},
			},
			want: VerdictAccept,
		},
		{
			name:            "Allow configured network by name",
			allowedNetworks: []string{"My Custom Net"},
			req: &domain.SandboxRequest{
				NetworkRef: domain.NetworkPolicyRef{ID: "id-123", Name: "My Custom Net"},
			},
			want: VerdictAccept,
		},
		{
			name:            "Reject unconfigured network",
			allowedNetworks: []string{"custom-net"},
			req: &domain.SandboxRequest{
				NetworkRef: domain.NetworkPolicyRef{ID: "evil-net"},
			},
			want: VerdictReject,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := NewNetworkJudge(tt.allowedNetworks, []netip.Prefix{}, logger)
			got, err := j.PreAdmit(context.Background(), tt.req)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
