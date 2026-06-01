/*
Copyright 2026 Paperclip Inc.

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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	reconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "openclaw_reconcile_total",
			Help: "Total number of reconciliations per instance",
		},
		[]string{"instance", "namespace", "result"},
	)

	reconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "openclaw_reconcile_duration_seconds",
			Help:    "Duration of reconciliation in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"instance", "namespace"},
	)

	instancePhase = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "openclaw_instance_phase",
			Help: "Current phase of an OpenClaw instance (1 = active for given phase)",
		},
		[]string{"instance", "namespace", "phase"},
	)

	resourceCreationFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "openclaw_resource_creation_failures_total",
			Help: "Total number of resource creation failures",
		},
		[]string{"instance", "namespace", "resource"},
	)

	managedInstances = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "openclaw_managed_instances",
			Help: "Current number of managed OpenClaw instances",
		},
	)

	autoUpdateChecksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "openclaw_autoupdate_checks_total",
			Help: "Total number of auto-update version checks",
		},
		[]string{"instance", "namespace", "result"},
	)

	autoUpdateAppliedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "openclaw_autoupdate_applied_total",
			Help: "Total number of auto-updates applied",
		},
		[]string{"instance", "namespace"},
	)

	autoUpdateRollbacksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "openclaw_autoupdate_rollbacks_total",
			Help: "Total number of auto-update rollbacks",
		},
		[]string{"instance", "namespace"},
	)

	instanceInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "openclaw_instance_info",
			Help: "Information about an OpenClaw instance (always 1, use for PromQL joins)",
		},
		[]string{"instance", "namespace", "version", "image"},
	)

	instanceReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "openclaw_instance_ready",
			Help: "Whether the OpenClaw instance pod is ready (1=ready, 0=not ready)",
		},
		[]string{"instance", "namespace"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		reconcileTotal,
		reconcileDuration,
		instancePhase,
		resourceCreationFailures,
		managedInstances,
		autoUpdateChecksTotal,
		autoUpdateAppliedTotal,
		autoUpdateRollbacksTotal,
		instanceInfo,
		instanceReady,
	)
}
