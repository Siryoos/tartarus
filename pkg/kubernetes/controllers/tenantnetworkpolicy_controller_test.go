/*
Copyright 2025 Tartarus Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	tartarusv1alpha1 "github.com/tartarus-sandbox/tartarus/pkg/kubernetes/apis/tartarus/v1alpha1"
)

func TestTenantNetworkPolicyReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(tartarusv1alpha1.AddToScheme(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))

	tests := []struct {
		name              string
		policy            *tartarusv1alpha1.TenantNetworkPolicy
		networkPolicy     *networkingv1.NetworkPolicy
		expectedReady     bool
		expectedAllowCIDR int
	}{
		{
			name: "valid policy with explicit tenant ID",
			policy: &tartarusv1alpha1.TenantNetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: "default",
				},
				Spec: tartarusv1alpha1.TenantNetworkPolicySpec{
					TenantID: "acme-corp",
					DefaultContract: tartarusv1alpha1.ContractSpec{
						AllowedCIDRs: []string{"10.0.0.0/8"},
						DenyPrivate:  true,
						DenyMetadata: true,
					},
				},
			},
			expectedReady:     true,
			expectedAllowCIDR: 1,
		},
		{
			name: "policy defaults to namespace as tenant ID",
			policy: &tartarusv1alpha1.TenantNetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: "tenant-xyz",
				},
				Spec: tartarusv1alpha1.TenantNetworkPolicySpec{
					DefaultContract: tartarusv1alpha1.ContractSpec{
						AllowedCIDRs: []string{"192.168.1.0/24", "10.20.30.0/24"},
						DenyMetadata: true,
					},
				},
			},
			expectedReady:     true,
			expectedAllowCIDR: 2,
		},
		{
			name: "policy with invalid CIDR",
			policy: &tartarusv1alpha1.TenantNetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-policy",
					Namespace: "default",
				},
				Spec: tartarusv1alpha1.TenantNetworkPolicySpec{
					DefaultContract: tartarusv1alpha1.ContractSpec{
						AllowedCIDRs: []string{"not-a-cidr"},
					},
				},
			},
			expectedReady:     false,
			expectedAllowCIDR: 0,
		},
		{
			name: "policy with NetworkPolicy reference",
			policy: &tartarusv1alpha1.TenantNetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ref-policy",
					Namespace: "default",
				},
				Spec: tartarusv1alpha1.TenantNetworkPolicySpec{
					TenantID: "ref-tenant",
					DefaultContract: tartarusv1alpha1.ContractSpec{
						AllowedCIDRs: []string{"10.0.0.0/8"},
					},
					NetworkPolicyRef: &tartarusv1alpha1.PolicyReference{
						Name: "k8s-netpol",
					},
				},
			},
			networkPolicy: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "k8s-netpol",
					Namespace: "default",
				},
				Spec: networkingv1.NetworkPolicySpec{
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
					Egress: []networkingv1.NetworkPolicyEgressRule{
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									IPBlock: &networkingv1.IPBlock{
										CIDR: "172.16.0.0/16",
									},
								},
							},
						},
					},
				},
			},
			expectedReady:     true,
			expectedAllowCIDR: 2, // 10.0.0.0/8 from spec + 172.16.0.0/16 from NetworkPolicy
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{tt.policy}
			if tt.networkPolicy != nil {
				objects = append(objects, tt.networkPolicy)
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objects...).
				WithStatusSubresource(&tartarusv1alpha1.TenantNetworkPolicy{}).
				Build()

			reconciler := &TenantNetworkPolicyReconciler{
				Client: client,
				Scheme: scheme,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.policy.Name,
					Namespace: tt.policy.Namespace,
				},
			}

			_, err := reconciler.Reconcile(context.Background(), req)
			require.NoError(t, err)

			// Check status
			updatedPolicy := &tartarusv1alpha1.TenantNetworkPolicy{}
			err = client.Get(context.Background(), req.NamespacedName, updatedPolicy)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedReady, updatedPolicy.Status.Ready)

			// Check cached contract
			if tt.expectedReady {
				tenantID := tt.policy.Spec.TenantID
				if tenantID == "" {
					tenantID = tt.policy.Namespace
				}
				contract := reconciler.GetContractForTenant(tt.policy.Namespace, tenantID)
				require.NotNil(t, contract, "Contract should be cached")
				assert.Equal(t, tt.expectedAllowCIDR, len(contract.AllowedCIDRs))
			}
		})
	}
}

func TestTenantNetworkPolicyReconciler_MissingNetworkPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(tartarusv1alpha1.AddToScheme(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))

	policy := &tartarusv1alpha1.TenantNetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: tartarusv1alpha1.TenantNetworkPolicySpec{
			NetworkPolicyRef: &tartarusv1alpha1.PolicyReference{
				Name: "missing-netpol",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(policy).
		WithStatusSubresource(&tartarusv1alpha1.TenantNetworkPolicy{}).
		Build()

	reconciler := &TenantNetworkPolicyReconciler{
		Client: client,
		Scheme: scheme,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      policy.Name,
			Namespace: policy.Namespace,
		},
	}

	_, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)

	// Check status - should not be ready
	updatedPolicy := &tartarusv1alpha1.TenantNetworkPolicy{}
	err = client.Get(context.Background(), req.NamespacedName, updatedPolicy)
	require.NoError(t, err)

	assert.False(t, updatedPolicy.Status.Ready)
	assert.Contains(t, updatedPolicy.Status.Message, "NetworkPolicy not found")
}
