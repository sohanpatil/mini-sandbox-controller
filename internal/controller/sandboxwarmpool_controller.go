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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	demov1alpha1 "example.com/mini-sandbox-controller/api/v1alpha1"
)

// SandboxWarmPoolReconciler reconciles a SandboxWarmPool object
type SandboxWarmPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxwarmpools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxwarmpools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxwarmpools/finalizers,verbs=update
// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxtemplates,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SandboxWarmPool object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *SandboxWarmPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	warmPool := &demov1alpha1.SandboxWarmPool{}
	if err := r.Get(ctx, req.NamespacedName, warmPool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	template := &demov1alpha1.SandboxTemplate{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      warmPool.Spec.TemplateRef.Name,
		Namespace: warmPool.Namespace,
	}, template); err != nil {
		return ctrl.Result{}, err
	}

	sandboxList := &demov1alpha1.SandboxList{}
	if err := r.List(ctx, sandboxList, client.InNamespace(warmPool.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	ownedSandboxes := []demov1alpha1.Sandbox{}
	readyReplicas := int32(0)

	for _, sandbox := range sandboxList.Items {
		owner := metav1.GetControllerOf(&sandbox)
		if owner == nil || owner.UID != warmPool.UID {
			continue
		}

		ownedSandboxes = append(ownedSandboxes, sandbox)
		if sandbox.Status.Phase == "Running" {
			readyReplicas++
		}
	}

	existingNames := map[string]struct{}{}
	for i := range ownedSandboxes {
		existingNames[ownedSandboxes[i].Name] = struct{}{}
	}

	for i := int32(len(ownedSandboxes)); i < warmPool.Spec.Replicas; i++ {
		sandboxName := nextWarmSandboxName(warmPool.Name, existingNames)
		sandbox := &demov1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sandboxName,
				Namespace: warmPool.Namespace,
				Labels: map[string]string{
					warmPoolLabel:    warmPool.Name,
					templateRefLabel: warmPool.Spec.TemplateRef.Name,
				},
			},
			Spec: demov1alpha1.SandboxSpec{
				Image:   &template.Spec.Image,
				Storage: template.Spec.Storage,
			},
		}

		if err := ctrl.SetControllerReference(warmPool, sandbox, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, sandbox); err != nil {
			return ctrl.Result{}, err
		}
		existingNames[sandboxName] = struct{}{}
		ownedSandboxes = append(ownedSandboxes, *sandbox)
	}

	if int32(len(ownedSandboxes)) > warmPool.Spec.Replicas {
		for i := warmPool.Spec.Replicas; i < int32(len(ownedSandboxes)); i++ {
			sandbox := &ownedSandboxes[i]
			if err := r.Delete(ctx, sandbox); err != nil {
				return ctrl.Result{}, err
			}
		}
		ownedSandboxes = ownedSandboxes[:warmPool.Spec.Replicas]
	}

	readyReplicas = countReadySandboxes(ownedSandboxes)
	if warmPool.Status.Replicas != int32(len(ownedSandboxes)) ||
		warmPool.Status.ReadyReplicas != readyReplicas {
		warmPool.Status.Replicas = int32(len(ownedSandboxes))
		warmPool.Status.ReadyReplicas = readyReplicas
		if err := r.Status().Update(ctx, warmPool); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func nextWarmSandboxName(poolName string, existingNames map[string]struct{}) string {
	prefix := poolName + "-"
	for i := int32(0); ; i++ {
		name := fmt.Sprintf("%s%d", prefix, i)
		if _, exists := existingNames[name]; !exists {
			return name
		}
	}
}

func countReadySandboxes(sandboxes []demov1alpha1.Sandbox) int32 {
	readyReplicas := int32(0)
	for i := range sandboxes {
		if sandboxes[i].Status.Phase == "Running" {
			readyReplicas++
		}
	}
	return readyReplicas
}

// SetupWithManager sets up the controller with the Manager.
func (r *SandboxWarmPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1alpha1.SandboxWarmPool{}).
		Owns(&demov1alpha1.Sandbox{}).
		Named("sandboxwarmpool").
		Complete(r)
}
