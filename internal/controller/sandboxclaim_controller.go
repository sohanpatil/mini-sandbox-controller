/*
Copyright 2026.

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

package controller

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	demov1alpha1 "example.com/mini-sandbox-controller/api/v1alpha1"
)

// SandboxClaimReconciler reconciles a SandboxClaim object
type SandboxClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxclaims/finalizers,verbs=update
// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxtemplates,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SandboxClaim object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *SandboxClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	claim := &demov1alpha1.SandboxClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	template := &demov1alpha1.SandboxTemplate{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      claim.Spec.TemplateRef.Name,
		Namespace: claim.Namespace,
	}, template); err != nil {
		return ctrl.Result{}, err
	}

	sandboxName := claim.Name
	if claim.Status.SandboxName != "" {
		sandboxName = claim.Status.SandboxName
	}

	sandbox := &demov1alpha1.Sandbox{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      sandboxName,
		Namespace: claim.Namespace,
	}, sandbox)

	if apierrors.IsNotFound(err) {
		warmSandbox, err := r.findAvailableWarmSandbox(ctx, claim)
		if err != nil {
			return ctrl.Result{}, err
		}

		if warmSandbox != nil {
			if err := r.adoptWarmSandbox(ctx, claim, warmSandbox); err != nil {
				return ctrl.Result{}, err
			}
			return r.updateStatus(ctx, claim, warmSandbox)
		}

		sandbox = &demov1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sandboxName,
				Namespace: claim.Namespace,
			},
			Spec: demov1alpha1.SandboxSpec{
				Image:   &template.Spec.Image,
				Storage: template.Spec.Storage,
			},
		}

		if err := ctrl.SetControllerReference(claim, sandbox, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, sandbox); err != nil {
			return ctrl.Result{}, err
		}

		return r.updateStatus(ctx, claim, sandbox)
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	return r.updateStatus(ctx, claim, sandbox)
}

func (r *SandboxClaimReconciler) adoptWarmSandbox(ctx context.Context, claim *demov1alpha1.SandboxClaim, sandbox *demov1alpha1.Sandbox) error {
	patch := client.MergeFrom(sandbox.DeepCopy())

	sandbox.OwnerReferences = nil
	if err := ctrl.SetControllerReference(claim, sandbox, r.Scheme); err != nil {
		return err
	}

	if sandbox.Labels == nil {
		sandbox.Labels = map[string]string{}
	}
	sandbox.Labels[claimLabel] = claim.Name

	return r.Patch(ctx, sandbox, patch)
}

func (r *SandboxClaimReconciler) findAvailableWarmSandbox(ctx context.Context, claim *demov1alpha1.SandboxClaim) (*demov1alpha1.Sandbox, error) {
	sandboxList := &demov1alpha1.SandboxList{}
	if err := r.List(ctx, sandboxList,
		client.InNamespace(claim.Namespace),
		client.MatchingLabels{templateRefLabel: claim.Spec.TemplateRef.Name},
	); err != nil {
		return nil, err
	}

	for i := range sandboxList.Items {
		sandbox := &sandboxList.Items[i]

		owner := metav1.GetControllerOf(sandbox)
		if owner == nil {
			continue
		}

		if owner.APIVersion != demov1alpha1.GroupVersion.String() ||
			owner.Kind != "SandboxWarmPool" {
			continue
		}

		if sandbox.Status.Phase != "Running" {
			continue
		}

		return sandbox, nil
	}

	return nil, nil
}

func (r *SandboxClaimReconciler) updateStatus(ctx context.Context, claim *demov1alpha1.SandboxClaim, sandbox *demov1alpha1.Sandbox) (ctrl.Result, error) {
	sandboxName := sandbox.Name
	phase := sandbox.Status.Phase

	if claim.Status.SandboxName == sandboxName &&
		claim.Status.Phase == phase {
		return ctrl.Result{}, nil
	}

	claim.Status.SandboxName = sandboxName
	claim.Status.Phase = phase

	return ctrl.Result{}, r.Status().Update(ctx, claim)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SandboxClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1alpha1.SandboxClaim{}).
		Owns(&demov1alpha1.Sandbox{}).
		Named("sandboxclaim").
		Complete(r)

}
