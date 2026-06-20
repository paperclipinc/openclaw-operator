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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

// retryableConflictResult classifies an error returned from resource
// reconciliation. A Kubernetes optimistic-lock conflict ("object has been
// modified") is a benign, retryable controller-runtime race rather than a
// terminal workload failure: it must be requeued promptly without producing a
// failed-looking OpenClawInstance status or a ReconcileFailed event. When err
// is such a conflict, it returns a prompt requeue and true; otherwise it
// returns the zero result and false so the caller handles the error normally.
//
// apierrors.IsConflict unwraps errors joined with %w, so conflicts surfaced
// through wrapped reconcile errors are still detected.
func retryableConflictResult(err error) (ctrl.Result, bool) {
	if apierrors.IsConflict(err) {
		return ctrl.Result{Requeue: true}, true
	}
	return ctrl.Result{}, false
}
