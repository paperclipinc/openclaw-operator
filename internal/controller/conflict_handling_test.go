/*
Copyright 2025.

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
	"errors"
	"fmt"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Regression for the optimistic-lock race described in issue #553: a benign
// "object has been modified" conflict during reconcile (for example a resume
// that updates StatefulSet/status from a slightly stale resourceVersion) must
// be treated as a retryable requeue, never as a terminal reconcile failure
// that emits ReconcileFailed / PhaseFailed for a healthy workload.
func TestRetryableConflictResult(t *testing.T) {
	gr := schema.GroupResource{Group: "apps", Resource: "statefulsets"}
	conflict := apierrors.NewConflict(gr, "test-instance", errors.New("object has been modified"))

	t.Run("bare optimistic-lock conflict requeues without failing", func(t *testing.T) {
		res, handled := retryableConflictResult(conflict)
		if !handled {
			t.Fatal("expected an optimistic-lock conflict to be handled as retryable")
		}
		if !res.Requeue {
			t.Errorf("expected a prompt requeue (Requeue=true), got %+v", res)
		}
	})

	t.Run("wrapped conflict is still detected", func(t *testing.T) {
		wrapped := fmt.Errorf("reconcile statefulset: %w", conflict)
		if _, handled := retryableConflictResult(wrapped); !handled {
			t.Error("expected a wrapped conflict to be handled as retryable")
		}
	})

	t.Run("non-conflict errors are not treated as retryable", func(t *testing.T) {
		notFound := apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, "missing")
		if _, handled := retryableConflictResult(notFound); handled {
			t.Error("a NotFound error must not be treated as a retryable conflict")
		}
		if _, handled := retryableConflictResult(errors.New("boom")); handled {
			t.Error("a generic error must not be treated as a retryable conflict")
		}
	})
}
