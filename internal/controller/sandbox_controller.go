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
	"time"

	demov1alpha1 "example.com/mini-sandbox-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// SandboxReconciler reconciles a Sandbox object
type SandboxReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=sandboxes/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Sandbox object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *SandboxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	sandbox := &demov1alpha1.Sandbox{}
	if err := r.Get(ctx, req.NamespacedName, sandbox); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if sandbox.Spec.Image == nil || *sandbox.Spec.Image == "" {
		return ctrl.Result{}, nil
	}

	expired, requeueAfter := sandboxExpired(sandbox, time.Now())
	if expired {
		if err := r.stopPod(ctx, sandbox); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.reconcileService(ctx, sandbox); err != nil {
			return ctrl.Result{}, err
		}
		return r.updateStatus(ctx, sandbox, "Expired")
	}

	if desiredReplicas(sandbox) == 0 {
		if err := r.stopPod(ctx, sandbox); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.reconcileService(ctx, sandbox); err != nil {
			return ctrl.Result{}, err
		}
		return r.updateStatus(ctx, sandbox, "Stopped")
	}

	if err := r.reconcilePVC(ctx, sandbox); err != nil {
		return ctrl.Result{}, err
	}

	pod, created, err := r.reconcilePod(ctx, sandbox)
	if err != nil {
		return ctrl.Result{}, err
	}
	if created {
		return ctrl.Result{}, nil
	}

	if err := r.reconcileService(ctx, sandbox); err != nil {
		return ctrl.Result{}, err
	}

	result, err := r.updateStatus(ctx, sandbox, string(pod.Status.Phase))
	if err != nil {
		return result, err
	}
	if requeueAfter > 0 {
		result.RequeueAfter = requeueAfter
	}
	return result, nil
}

func desiredReplicas(sandbox *demov1alpha1.Sandbox) int32 {
	if sandbox.Spec.Replicas == nil {
		return 1
	}
	return *sandbox.Spec.Replicas
}

func sandboxExpired(sandbox *demov1alpha1.Sandbox, now time.Time) (bool, time.Duration) {
	if sandbox.Spec.ShutdownTime == nil {
		return false, 0
	}

	shutdownTime := sandbox.Spec.ShutdownTime.Time
	if !now.Before(shutdownTime) {
		return true, 0
	}

	return false, shutdownTime.Sub(now)
}

func (r *SandboxReconciler) reconcilePod(ctx context.Context, sandbox *demov1alpha1.Sandbox) (*corev1.Pod, bool, error) {
	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      sandbox.Name,
		Namespace: sandbox.Namespace,
	}, pod)

	if apierrors.IsNotFound(err) {
		log := logf.FromContext(ctx)
		pod = podForSandbox(sandbox, *sandbox.Spec.Image)
		if err := ctrl.SetControllerReference(sandbox, pod, r.Scheme); err != nil {
			return nil, false, err
		}
		log.Info("creating pod for sandbox", "pod", pod.Name, "image", *sandbox.Spec.Image)
		if err := r.Create(ctx, pod); err != nil {
			return nil, false, err
		}
		return pod, true, nil
	}

	if err != nil {
		return nil, false, err
	}

	return pod, false, nil
}

func podForSandbox(sandbox *demov1alpha1.Sandbox, image string) *corev1.Pod {
	container := corev1.Container{
		Name:  "main",
		Image: image,
	}

	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Containers: []corev1.Container{
			container,
		},
	}

	if sandbox.Spec.Storage != nil {
		volumeName := "data"
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcNameForSandbox(sandbox),
				},
			},
		})

		podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: storageMountPath(sandbox),
		})
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sandbox.Name,
			Namespace: sandbox.Namespace,
			Labels:    labelsForSandbox(sandbox),
		},
		Spec: podSpec,
	}
}

func labelsForSandbox(sandbox *demov1alpha1.Sandbox) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "mini-sandbox",
		"app.kubernetes.io/managed-by": "mini-sandbox-controller",
		sandboxNameLabel:               sandbox.Name,
	}
}

func storageMountPath(sandbox *demov1alpha1.Sandbox) string {
	if sandbox.Spec.Storage == nil || sandbox.Spec.Storage.MountPath == "" {
		return "/data"
	}
	return sandbox.Spec.Storage.MountPath
}

func pvcNameForSandbox(sandbox *demov1alpha1.Sandbox) string {
	return sandbox.Name + "-data"
}

func (r *SandboxReconciler) reconcilePVC(ctx context.Context, sandbox *demov1alpha1.Sandbox) error {
	if sandbox.Spec.Storage == nil {
		return nil
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      pvcNameForSandbox(sandbox),
		Namespace: sandbox.Namespace,
	}, pvc)

	if apierrors.IsNotFound(err) {
		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcNameForSandbox(sandbox),
				Namespace: sandbox.Namespace,
				Labels:    labelsForSandbox(sandbox),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(sandbox.Spec.Storage.Size),
					},
				},
			},
		}

		if err := ctrl.SetControllerReference(sandbox, pvc, r.Scheme); err != nil {
			return err
		}

		return r.Create(ctx, pvc)
	}

	return err
}

func (r *SandboxReconciler) stopPod(ctx context.Context, sandbox *demov1alpha1.Sandbox) error {
	log := logf.FromContext(ctx)
	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      sandbox.Name,
		Namespace: sandbox.Namespace,
	}, pod)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	log.Info("deleting pod because replicas is 0", "pod", pod.Name)
	return r.Delete(ctx, pod)
}

func (r *SandboxReconciler) reconcileService(ctx context.Context, sandbox *demov1alpha1.Sandbox) error {
	service := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      sandbox.Name,
		Namespace: sandbox.Namespace,
	}, service)

	if apierrors.IsNotFound(err) {
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sandbox.Name,
				Namespace: sandbox.Namespace,
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: corev1.ClusterIPNone,
				Selector: map[string]string{
					sandboxNameLabel: sandbox.Name,
				},
			},
		}

		if err := ctrl.SetControllerReference(sandbox, service, r.Scheme); err != nil {
			return err
		}

		if err := r.Create(ctx, service); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}

func (r *SandboxReconciler) updateStatus(ctx context.Context, sandbox *demov1alpha1.Sandbox, phase string) (ctrl.Result, error) {
	// When a pod is created that is owned by Sandbox, Phase remains the same (Creating)
	// Once pod creation completes, the SandboxController Reconcile is run again (as we watch for Pod CRUD owned by SandBox)
	serviceName := sandbox.Name
	serviceDNS := serviceName + "." + sandbox.Namespace + ".svc.cluster.local"

	if sandbox.Status.Phase == phase &&
		sandbox.Status.ServiceName == serviceName &&
		sandbox.Status.ServiceDNS == serviceDNS {
		return ctrl.Result{}, nil
	}

	sandbox.Status.Phase = phase
	sandbox.Status.ServiceName = serviceName
	sandbox.Status.ServiceDNS = serviceDNS

	return ctrl.Result{}, r.Status().Update(ctx, sandbox)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SandboxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1alpha1.Sandbox{}). // Reconcile Sandboxes when Sandboxes change,
		// and also reconcile the relevant Sandbox when one of its owned Pods changes.
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Named("sandbox").
		Complete(r)
}
