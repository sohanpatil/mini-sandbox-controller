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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	demov1alpha1 "example.com/mini-sandbox-controller/api/v1alpha1"
)

func TestDesiredReplicasDefaultsToOne(t *testing.T) {
	sandbox := &demov1alpha1.Sandbox{}

	if got := desiredReplicas(sandbox); got != 1 {
		t.Fatalf("desiredReplicas() = %d, want 1", got)
	}
}

func TestDesiredReplicasUsesSpecValue(t *testing.T) {
	replicas := int32(0)
	sandbox := &demov1alpha1.Sandbox{
		Spec: demov1alpha1.SandboxSpec{
			Replicas: &replicas,
		},
	}

	if got := desiredReplicas(sandbox); got != 0 {
		t.Fatalf("desiredReplicas() = %d, want 0", got)
	}
}

func TestSandboxExpired(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	shutdownTime := metav1.NewTime(now.Add(-time.Minute))
	sandbox := &demov1alpha1.Sandbox{
		Spec: demov1alpha1.SandboxSpec{
			ShutdownTime: &shutdownTime,
		},
	}

	expired, requeueAfter := sandboxExpired(sandbox, now)
	if !expired {
		t.Fatal("sandboxExpired() expired = false, want true")
	}
	if requeueAfter != 0 {
		t.Fatalf("sandboxExpired() requeueAfter = %s, want 0", requeueAfter)
	}
}

func TestSandboxExpiredRequeuesUntilShutdown(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	shutdownTime := metav1.NewTime(now.Add(5 * time.Minute))
	sandbox := &demov1alpha1.Sandbox{
		Spec: demov1alpha1.SandboxSpec{
			ShutdownTime: &shutdownTime,
		},
	}

	expired, requeueAfter := sandboxExpired(sandbox, now)
	if expired {
		t.Fatal("sandboxExpired() expired = true, want false")
	}
	if requeueAfter != 5*time.Minute {
		t.Fatalf("sandboxExpired() requeueAfter = %s, want 5m", requeueAfter)
	}
}

func TestNextWarmSandboxNameSkipsExistingNames(t *testing.T) {
	existingNames := map[string]struct{}{
		"python-pool-0": {},
		"python-pool-1": {},
	}

	if got := nextWarmSandboxName("python-pool", existingNames); got != "python-pool-2" {
		t.Fatalf("nextWarmSandboxName() = %q, want %q", got, "python-pool-2")
	}
}
