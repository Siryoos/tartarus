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
	"net/netip"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1 "k8s.io/api/networking/v1"

	"github.com/tartarus-sandbox/tartarus/pkg/styx"

	tartarusv1alpha1 "github.com/tartarus-sandbox/tartarus/pkg/kubernetes/apis/tartarus/v1alpha1"
)

// TenantNetworkPolicyReconciler reconciles a TenantNetworkPolicy object
type TenantNetworkPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// contractCache caches resolved contracts per tenant for quick lookup
	contractCache map[string]*styx.Contract
	cacheMu       sync.RWMutex
}

//+kubebuilder:rbac:groups=tartarus.io,resources=tenantnetworkpolicies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=tartarus.io,resources=tenantnetworkpolicies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=tartarus.io,resources=tenantnetworkpolicies/finalizers,verbs=update
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *TenantNetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var policy tartarusv1alpha1.TenantNetworkPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		// Remove from cache if deleted
		r.cacheMu.Lock()
		delete(r.contractCache, r.getTenantKey(req.Namespace, ""))
		r.cacheMu.Unlock()
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Determine tenant ID
	tenantID := policy.Spec.TenantID
	if tenantID == "" {
		tenantID = policy.Namespace
	}

	// Build contract from spec
	contract := &styx.Contract{
		ID:           tenantID,
		DenyPrivate:  policy.Spec.DefaultContract.DenyPrivate,
		DenyMetadata: policy.Spec.DefaultContract.DenyMetadata,
	}

	// Parse allowed CIDRs
	for _, cidrStr := range policy.Spec.DefaultContract.AllowedCIDRs {
		prefix, err := netip.ParsePrefix(cidrStr)
		if err != nil {
			logger.Error(err, "Invalid CIDR in allowedCidrs", "cidr", cidrStr)
			policy.Status.Ready = false
			policy.Status.Message = "Invalid CIDR: " + cidrStr

			meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
				Type:    string(tartarusv1alpha1.TenantNetworkPolicyReady),
				Status:  metav1.ConditionFalse,
				Reason:  "InvalidCIDR",
				Message: "Invalid CIDR: " + cidrStr,
			})

			if err := r.Status().Update(ctx, &policy); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		contract.AllowedCIDRs = append(contract.AllowedCIDRs, prefix)
	}

	// Resolve referenced K8s NetworkPolicy if specified
	if policy.Spec.NetworkPolicyRef != nil {
		netPolNamespace := policy.Spec.NetworkPolicyRef.Namespace
		if netPolNamespace == "" {
			netPolNamespace = policy.Namespace
		}

		var netPol networkingv1.NetworkPolicy
		netPolKey := types.NamespacedName{
			Name:      policy.Spec.NetworkPolicyRef.Name,
			Namespace: netPolNamespace,
		}

		if err := r.Get(ctx, netPolKey, &netPol); err != nil {
			logger.Error(err, "Failed to resolve referenced NetworkPolicy", "name", policy.Spec.NetworkPolicyRef.Name)
			policy.Status.Ready = false
			policy.Status.Message = "NetworkPolicy not found: " + policy.Spec.NetworkPolicyRef.Name

			meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
				Type:    string(tartarusv1alpha1.TenantNetworkPolicyNetworkPolicyResolved),
				Status:  metav1.ConditionFalse,
				Reason:  "NotFound",
				Message: "NetworkPolicy not found: " + policy.Spec.NetworkPolicyRef.Name,
			})

			meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
				Type:    string(tartarusv1alpha1.TenantNetworkPolicyReady),
				Status:  metav1.ConditionFalse,
				Reason:  "NetworkPolicyNotFound",
				Message: "Referenced NetworkPolicy not found",
			})

			if err := r.Status().Update(ctx, &policy); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		// Translate K8s NetworkPolicy to Styx contract
		r.translateNetworkPolicy(&netPol, contract)

		meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
			Type:    string(tartarusv1alpha1.TenantNetworkPolicyNetworkPolicyResolved),
			Status:  metav1.ConditionTrue,
			Reason:  "Resolved",
			Message: "NetworkPolicy resolved successfully",
		})
	}

	// Cache the contract
	r.cacheMu.Lock()
	if r.contractCache == nil {
		r.contractCache = make(map[string]*styx.Contract)
	}
	r.contractCache[r.getTenantKey(policy.Namespace, tenantID)] = contract
	r.cacheMu.Unlock()

	// Update status
	now := metav1.Now()
	policy.Status.Ready = true
	policy.Status.Message = "Policy is ready"
	policy.Status.LastApplied = &now

	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:    string(tartarusv1alpha1.TenantNetworkPolicyReady),
		Status:  metav1.ConditionTrue,
		Reason:  "Valid",
		Message: "Policy is valid and cached",
	})

	if err := r.Status().Update(ctx, &policy); err != nil {
		logger.Error(err, "Failed to update TenantNetworkPolicy status")
		return ctrl.Result{}, err
	}

	logger.Info("TenantNetworkPolicy reconciled", "tenant", tenantID, "allowedCIDRs", len(contract.AllowedCIDRs))
	return ctrl.Result{}, nil
}

// translateNetworkPolicy translates a K8s NetworkPolicy to Styx contract rules
func (r *TenantNetworkPolicyReconciler) translateNetworkPolicy(netPol *networkingv1.NetworkPolicy, contract *styx.Contract) {
	// Extract allowed CIDRs from egress rules
	for _, egress := range netPol.Spec.Egress {
		for _, to := range egress.To {
			if to.IPBlock != nil {
				prefix, err := netip.ParsePrefix(to.IPBlock.CIDR)
				if err == nil {
					contract.AllowedCIDRs = append(contract.AllowedCIDRs, prefix)
				}
			}
		}
	}

	// If no egress rules defined but PolicyTypes includes Egress, deny all by default
	hasEgressPolicy := false
	for _, pt := range netPol.Spec.PolicyTypes {
		if pt == networkingv1.PolicyTypeEgress {
			hasEgressPolicy = true
			break
		}
	}

	if hasEgressPolicy && len(netPol.Spec.Egress) == 0 {
		// Egress policy with no rules = deny all
		contract.DenyPrivate = true
		contract.DenyMetadata = true
	}
}

// GetContractForTenant retrieves the cached contract for a tenant
func (r *TenantNetworkPolicyReconciler) GetContractForTenant(namespace, tenantID string) *styx.Contract {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	if r.contractCache == nil {
		return nil
	}

	// Try explicit tenant ID first
	if tenantID != "" {
		if contract, ok := r.contractCache[r.getTenantKey(namespace, tenantID)]; ok {
			return contract
		}
	}

	// Fall back to namespace-based lookup
	if contract, ok := r.contractCache[r.getTenantKey(namespace, "")]; ok {
		return contract
	}

	return nil
}

// getTenantKey generates a cache key for tenant lookup
func (r *TenantNetworkPolicyReconciler) getTenantKey(namespace, tenantID string) string {
	if tenantID != "" {
		return tenantID
	}
	return namespace
}

// SetupWithManager sets up the controller with the Manager.
func (r *TenantNetworkPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tartarusv1alpha1.TenantNetworkPolicy{}).
		Complete(r)
}
