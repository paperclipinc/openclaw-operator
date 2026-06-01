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

package resources

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// HPAName returns the name of the HorizontalPodAutoscaler
func HPAName(instance *openclawv1alpha1.OpenClawInstance) string {
	return instance.Name
}

// IsHPAEnabled returns true if auto-scaling is enabled for the instance
func IsHPAEnabled(instance *openclawv1alpha1.OpenClawInstance) bool {
	return instance.Spec.Availability.AutoScaling != nil &&
		instance.Spec.Availability.AutoScaling.Enabled != nil &&
		*instance.Spec.Availability.AutoScaling.Enabled
}

// BuildHPA creates a HorizontalPodAutoscaler for the OpenClawInstance
func BuildHPA(instance *openclawv1alpha1.OpenClawInstance) *autoscalingv2.HorizontalPodAutoscaler {
	labels := Labels(instance)
	as := instance.Spec.Availability.AutoScaling

	// Defaults
	minReplicas := int32(1)
	if as.MinReplicas != nil {
		minReplicas = *as.MinReplicas
	}

	maxReplicas := int32(5)
	if as.MaxReplicas != nil {
		maxReplicas = *as.MaxReplicas
	}

	cpuTarget := int32(80)
	if as.TargetCPUUtilization != nil {
		cpuTarget = *as.TargetCPUUtilization
	}

	metrics := []autoscalingv2.MetricSpec{
		{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: Ptr(cpuTarget),
				},
			},
		},
	}

	// Optional memory metric
	if as.TargetMemoryUtilization != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: as.TargetMemoryUtilization,
				},
			},
		})
	}

	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      HPAName(instance),
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
				Name:       StatefulSetName(instance),
			},
			MinReplicas: Ptr(minReplicas),
			MaxReplicas: maxReplicas,
			Metrics:     metrics,
		},
	}
}
