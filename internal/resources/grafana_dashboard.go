/*
Copyright 2026 OpenClaw.rocks

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
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

const defaultGrafanaFolder = "OpenClaw"

// GrafanaDashboardOperatorName returns the name of the operator overview dashboard ConfigMap
func GrafanaDashboardOperatorName(instance *openclawv1alpha1.OpenClawInstance) string {
	return instance.Name + "-dashboard-operator"
}

// GrafanaDashboardInstanceName returns the name of the instance detail dashboard ConfigMap
func GrafanaDashboardInstanceName(instance *openclawv1alpha1.OpenClawInstance) string {
	return instance.Name + "-dashboard-instance"
}

// BuildGrafanaDashboardOperator creates a ConfigMap containing the operator overview Grafana dashboard
func BuildGrafanaDashboardOperator(instance *openclawv1alpha1.OpenClawInstance) *corev1.ConfigMap {
	dashboardJSON := buildOperatorDashboard()
	return buildDashboardConfigMap(instance, GrafanaDashboardOperatorName(instance), "openclaw-operator.json", dashboardJSON)
}

// BuildGrafanaDashboardInstance creates a ConfigMap containing the per-instance Grafana dashboard
func BuildGrafanaDashboardInstance(instance *openclawv1alpha1.OpenClawInstance) *corev1.ConfigMap {
	dashboardJSON := buildInstanceDashboard()
	return buildDashboardConfigMap(instance, GrafanaDashboardInstanceName(instance), "openclaw-instance.json", dashboardJSON)
}

func buildDashboardConfigMap(instance *openclawv1alpha1.OpenClawInstance, name, dataKey, dashboardJSON string) *corev1.ConfigMap {
	labels := Labels(instance)
	labels["grafana_dashboard"] = "1"

	folder := defaultGrafanaFolder
	if instance.Spec.Observability.Metrics.GrafanaDashboard != nil {
		if instance.Spec.Observability.Metrics.GrafanaDashboard.Folder != "" {
			folder = instance.Spec.Observability.Metrics.GrafanaDashboard.Folder
		}
		for k, v := range instance.Spec.Observability.Metrics.GrafanaDashboard.Labels {
			labels[k] = v
		}
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"grafana_folder": folder,
			},
		},
		Data: map[string]string{
			dataKey: dashboardJSON,
		},
	}
}

// --- Dashboard JSON builders ---

// grafanaDashboard represents a Grafana dashboard model
type grafanaDashboard struct {
	Annotations   grafanaAnnotations `json:"annotations"`
	Editable      bool               `json:"editable"`
	GraphTooltip  int                `json:"graphTooltip"`
	Panels        []grafanaPanel     `json:"panels"`
	SchemaVersion int                `json:"schemaVersion"`
	Tags          []string           `json:"tags"`
	Templating    grafanaTemplating  `json:"templating"`
	Time          grafanaTime        `json:"time"`
	Refresh       string             `json:"refresh"`
	Title         string             `json:"title"`
	UID           string             `json:"uid"`
}

type grafanaAnnotations struct {
	List []interface{} `json:"list"`
}

type grafanaTemplating struct {
	List []grafanaVariable `json:"list"`
}

type grafanaVariable struct {
	Current    map[string]interface{} `json:"current"`
	Hide       int                    `json:"hide"`
	IncludeAll bool                   `json:"includeAll"`
	Label      string                 `json:"label"`
	Multi      bool                   `json:"multi"`
	Name       string                 `json:"name"`
	Options    []interface{}          `json:"options"`
	Query      interface{}            `json:"query"`
	Refresh    int                    `json:"refresh"`
	Regex      string                 `json:"regex"`
	Type       string                 `json:"type"`
	Datasource interface{}            `json:"datasource,omitempty"`
	Definition string                 `json:"definition,omitempty"`
	Sort       int                    `json:"sort,omitempty"`
	AllValue   string                 `json:"allValue,omitempty"`
}

type grafanaTime struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type grafanaPanel struct {
	ID          int                    `json:"id"`
	Title       string                 `json:"title"`
	Type        string                 `json:"type"`
	GridPos     grafanaGridPos         `json:"gridPos"`
	Targets     []grafanaTarget        `json:"targets,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
	FieldConfig *grafanaFieldConfig    `json:"fieldConfig,omitempty"`
	Datasource  *grafanaDatasource     `json:"datasource,omitempty"`
	Panels      []grafanaPanel         `json:"panels,omitempty"`
	Collapsed   bool                   `json:"collapsed,omitempty"`
}

type grafanaGridPos struct {
	H int `json:"h"`
	W int `json:"w"`
	X int `json:"x"`
	Y int `json:"y"`
}

type grafanaTarget struct {
	Expr         string `json:"expr"`
	LegendFormat string `json:"legendFormat"`
	RefID        string `json:"refId"`
	Instant      bool   `json:"instant,omitempty"`
	Format       string `json:"format,omitempty"`
}

type grafanaFieldConfig struct {
	Defaults  map[string]interface{}   `json:"defaults"`
	Overrides []map[string]interface{} `json:"overrides,omitempty"`
}

type grafanaDatasource struct {
	Type string `json:"type"`
	UID  string `json:"uid"`
}

func dsVar() *grafanaDatasource {
	return &grafanaDatasource{Type: "prometheus", UID: "${datasource}"}
}

func datasourceVar() grafanaVariable {
	return grafanaVariable{
		Current: map[string]interface{}{},
		Hide:    0,
		Label:   "Datasource",
		Name:    "datasource",
		Options: []interface{}{},
		Query:   "prometheus",
		Refresh: 1,
		Type:    "datasource",
	}
}

func namespaceVar(multi bool) grafanaVariable {
	return grafanaVariable{
		Current:    map[string]interface{}{},
		Hide:       0,
		IncludeAll: multi,
		Label:      "Namespace",
		Multi:      multi,
		Name:       "namespace",
		Options:    []interface{}{},
		Query:      `label_values(openclaw_instance_info, namespace)`,
		Definition: `label_values(openclaw_instance_info, namespace)`,
		Refresh:    2,
		Type:       "query",
		Sort:       1,
		Datasource: map[string]interface{}{"type": "prometheus", "uid": "${datasource}"},
	}
}

func instanceVar(multi bool) grafanaVariable {
	v := grafanaVariable{
		Current:    map[string]interface{}{},
		Hide:       0,
		IncludeAll: multi,
		Label:      "Instance",
		Multi:      multi,
		Name:       "instance",
		Options:    []interface{}{},
		Query:      `label_values(openclaw_instance_info{namespace=~"$namespace"}, instance)`,
		Definition: `label_values(openclaw_instance_info{namespace=~"$namespace"}, instance)`,
		Refresh:    2,
		Type:       "query",
		Sort:       1,
		Datasource: map[string]interface{}{"type": "prometheus", "uid": "${datasource}"},
	}
	if multi {
		v.AllValue = ".*"
	}
	return v
}

func statPanel(id int, title, expr string, pos grafanaGridPos) grafanaPanel {
	return grafanaPanel{
		ID:          id,
		Title:       title,
		Type:        "stat",
		GridPos:     pos,
		Targets:     []grafanaTarget{{Expr: expr, RefID: "A", Instant: true}},
		Datasource:  dsVar(),
		FieldConfig: &grafanaFieldConfig{Defaults: map[string]interface{}{}},
	}
}

func timeseriesPanel(id int, title string, targets []grafanaTarget, pos grafanaGridPos) grafanaPanel {
	return grafanaPanel{
		ID:         id,
		Title:      title,
		Type:       "timeseries",
		GridPos:    pos,
		Targets:    targets,
		Datasource: dsVar(),
		FieldConfig: &grafanaFieldConfig{Defaults: map[string]interface{}{
			"custom": map[string]interface{}{
				"lineWidth":   1,
				"fillOpacity": 10,
				"pointSize":   5,
				"showPoints":  "auto",
			},
		}},
	}
}

func gaugePanel(id int, title, expr string, pos grafanaGridPos) grafanaPanel {
	return grafanaPanel{
		ID:         id,
		Title:      title,
		Type:       "gauge",
		GridPos:    pos,
		Targets:    []grafanaTarget{{Expr: expr, RefID: "A", Instant: true}},
		Datasource: dsVar(),
		FieldConfig: &grafanaFieldConfig{Defaults: map[string]interface{}{
			"max": 1,
			"min": 0,
			"thresholds": map[string]interface{}{
				"steps": []map[string]interface{}{
					{"color": "green", "value": nil},
					{"color": "yellow", "value": 0.8},
					{"color": "red", "value": 0.9},
				},
			},
			"unit": "percentunit",
		}},
	}
}

func tablePanel(id int, title string, targets []grafanaTarget, pos grafanaGridPos) grafanaPanel {
	return grafanaPanel{
		ID:          id,
		Title:       title,
		Type:        "table",
		GridPos:     pos,
		Targets:     targets,
		Datasource:  dsVar(),
		FieldConfig: &grafanaFieldConfig{Defaults: map[string]interface{}{}},
		Options: map[string]interface{}{
			"showHeader": true,
			"sortBy":     []map[string]interface{}{{"displayName": "instance", "desc": false}},
		},
	}
}

func rowPanel(id int, title string, y int, collapsed bool, panels []grafanaPanel) grafanaPanel {
	return grafanaPanel{
		ID:        id,
		Title:     title,
		Type:      "row",
		GridPos:   grafanaGridPos{H: 1, W: 24, X: 0, Y: y},
		Collapsed: collapsed,
		Panels:    panels,
	}
}

func mustMarshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		// This should never happen with our static dashboard structures
		panic(err)
	}
	return string(b)
}

// --- Operator dashboard ---

func buildOperatorDashboard() string {
	dashboard := grafanaDashboard{
		Annotations:   grafanaAnnotations{List: []interface{}{}},
		Editable:      true,
		GraphTooltip:  1,
		SchemaVersion: 39,
		Tags:          []string{"openclaw", "operator"},
		Time:          grafanaTime{From: "now-1h", To: "now"},
		Refresh:       "30s",
		Title:         "OpenClaw Operator",
		UID:           "openclaw-operator-overview",
		Templating: grafanaTemplating{
			List: []grafanaVariable{
				datasourceVar(),
				namespaceVar(true),
				instanceVar(true),
			},
		},
		Panels: buildOperatorPanels(),
	}
	return mustMarshalJSON(dashboard)
}

func buildOperatorPanels() []grafanaPanel {
	gp := func(h, w, x, y int) grafanaGridPos { return grafanaGridPos{H: h, W: w, X: x, Y: y} }

	panels := []grafanaPanel{
		// --- Overview row ---
		rowPanel(100, "Overview", 0, false, nil),
		statPanel(1, "Managed Instances",
			`openclaw_managed_instances`, gp(4, 6, 0, 1)),
		statPanel(2, "Instances Ready",
			`count(openclaw_instance_ready{namespace=~"$namespace"} == 1)`, gp(4, 6, 6, 1)),
		statPanel(3, "Reconcile Error Rate",
			`sum(rate(openclaw_reconcile_total{result="error",namespace=~"$namespace"}[5m])) / clamp_min(sum(rate(openclaw_reconcile_total{namespace=~"$namespace"}[5m])), 1)`,
			gp(4, 6, 12, 1)),
		statPanel(4, "Resource Creation Failures",
			`sum(increase(openclaw_resource_creation_failures_total{namespace=~"$namespace"}[5m]))`,
			gp(4, 6, 18, 1)),

		// --- Reconciliation row ---
		rowPanel(101, "Reconciliation", 5, false, nil),
		timeseriesPanel(5, "Reconciliation Rate",
			[]grafanaTarget{
				{Expr: `sum(rate(openclaw_reconcile_total{result="success",namespace=~"$namespace",instance=~"$instance"}[5m])) by (instance)`, LegendFormat: "{{ instance }} - success", RefID: "A"},
				{Expr: `sum(rate(openclaw_reconcile_total{result="error",namespace=~"$namespace",instance=~"$instance"}[5m])) by (instance)`, LegendFormat: "{{ instance }} - error", RefID: "B"},
			}, gp(8, 12, 0, 6)),
		timeseriesPanel(6, "Reconciliation Duration",
			[]grafanaTarget{
				{Expr: `histogram_quantile(0.50, sum(rate(openclaw_reconcile_duration_seconds_bucket{namespace=~"$namespace",instance=~"$instance"}[5m])) by (le))`, LegendFormat: "p50", RefID: "A"},
				{Expr: `histogram_quantile(0.95, sum(rate(openclaw_reconcile_duration_seconds_bucket{namespace=~"$namespace",instance=~"$instance"}[5m])) by (le))`, LegendFormat: "p95", RefID: "B"},
				{Expr: `histogram_quantile(0.99, sum(rate(openclaw_reconcile_duration_seconds_bucket{namespace=~"$namespace",instance=~"$instance"}[5m])) by (le))`, LegendFormat: "p99", RefID: "C"},
			}, gp(8, 12, 12, 6)),

		// --- Instance Fleet row ---
		rowPanel(102, "Instance Fleet", 14, false, nil),
		tablePanel(7, "Instance Table",
			[]grafanaTarget{
				{Expr: `openclaw_instance_info{namespace=~"$namespace",instance=~"$instance"}`, LegendFormat: "", RefID: "A", Instant: true, Format: "table"},
				{Expr: `openclaw_instance_phase{namespace=~"$namespace",instance=~"$instance"} == 1`, LegendFormat: "", RefID: "B", Instant: true, Format: "table"},
				{Expr: `openclaw_instance_ready{namespace=~"$namespace",instance=~"$instance"}`, LegendFormat: "", RefID: "C", Instant: true, Format: "table"},
			}, gp(8, 24, 0, 15)),

		// --- Workqueue row (collapsed) ---
		rowPanel(103, "Workqueue", 23, true, []grafanaPanel{
			timeseriesPanel(8, "Queue Depth",
				[]grafanaTarget{
					{Expr: `workqueue_depth{name="openclawinstance"}`, LegendFormat: "depth", RefID: "A"},
				}, gp(8, 12, 0, 24)),
			timeseriesPanel(9, "Queue Wait Duration p99",
				[]grafanaTarget{
					{Expr: `histogram_quantile(0.99, sum(rate(workqueue_queue_duration_seconds_bucket{name="openclawinstance"}[5m])) by (le))`, LegendFormat: "p99", RefID: "A"},
				}, gp(8, 12, 12, 24)),
		}),

		// --- Kubernetes API row (collapsed) ---
		rowPanel(104, "Kubernetes API", 32, true, []grafanaPanel{
			timeseriesPanel(10, "API Requests",
				[]grafanaTarget{
					{Expr: `sum(rate(rest_client_requests_total[5m])) by (code)`, LegendFormat: "{{ code }}", RefID: "A"},
				}, gp(8, 24, 0, 33)),
		}),

		// --- Auto-Updates row (collapsed) ---
		rowPanel(105, "Auto-Updates", 41, true, []grafanaPanel{
			timeseriesPanel(11, "Update Checks",
				[]grafanaTarget{
					{Expr: `sum(rate(openclaw_autoupdate_checks_total{namespace=~"$namespace"}[5m])) by (result)`, LegendFormat: "{{ result }}", RefID: "A"},
				}, gp(8, 8, 0, 42)),
			timeseriesPanel(12, "Updates Applied",
				[]grafanaTarget{
					{Expr: `sum(increase(openclaw_autoupdate_applied_total{namespace=~"$namespace"}[1h])) by (instance)`, LegendFormat: "{{ instance }}", RefID: "A"},
				}, gp(8, 8, 8, 42)),
			timeseriesPanel(13, "Rollbacks",
				[]grafanaTarget{
					{Expr: `sum(increase(openclaw_autoupdate_rollbacks_total{namespace=~"$namespace"}[1h])) by (instance)`, LegendFormat: "{{ instance }}", RefID: "A"},
				}, gp(8, 8, 16, 42)),
		}),
	}
	return panels
}

// --- Instance dashboard ---

func buildInstanceDashboard() string {
	dashboard := grafanaDashboard{
		Annotations:   grafanaAnnotations{List: []interface{}{}},
		Editable:      true,
		GraphTooltip:  1,
		SchemaVersion: 39,
		Tags:          []string{"openclaw", "instance"},
		Time:          grafanaTime{From: "now-1h", To: "now"},
		Refresh:       "30s",
		Title:         "OpenClaw Instance",
		UID:           "openclaw-instance-detail",
		Templating: grafanaTemplating{
			List: []grafanaVariable{
				datasourceVar(),
				namespaceVar(false),
				instanceVar(false),
			},
		},
		Panels: buildInstancePanels(),
	}
	return mustMarshalJSON(dashboard)
}

func buildInstancePanels() []grafanaPanel {
	gp := func(h, w, x, y int) grafanaGridPos { return grafanaGridPos{H: h, W: w, X: x, Y: y} }

	panels := []grafanaPanel{
		// --- Health row ---
		rowPanel(200, "Health", 0, false, nil),
		statPanel(21, "Phase",
			`openclaw_instance_phase{namespace="$namespace",instance="$instance"} == 1`,
			gp(4, 5, 0, 1)),
		statPanel(22, "Ready",
			`openclaw_instance_ready{namespace="$namespace",instance="$instance"}`,
			gp(4, 5, 5, 1)),
		gaugePanel(23, "CPU %",
			`sum(rate(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$instance-.*",container="openclaw"}[5m])) / sum(kube_pod_container_resource_limits{namespace="$namespace",pod=~"$instance-.*",container="openclaw",resource="cpu"})`,
			gp(4, 5, 10, 1)),
		gaugePanel(24, "Memory %",
			`sum(container_memory_working_set_bytes{namespace="$namespace",pod=~"$instance-.*",container="openclaw"}) / sum(kube_pod_container_resource_limits{namespace="$namespace",pod=~"$instance-.*",container="openclaw",resource="memory"})`,
			gp(4, 5, 15, 1)),
		gaugePanel(25, "PVC %",
			`kubelet_volume_stats_used_bytes{namespace="$namespace",persistentvolumeclaim=~"data-$instance.*"} / kubelet_volume_stats_capacity_bytes{namespace="$namespace",persistentvolumeclaim=~"data-$instance.*"}`,
			gp(4, 4, 20, 1)),

		// --- CPU row ---
		rowPanel(201, "CPU", 5, false, nil),
		timeseriesPanel(26, "CPU Usage vs Request/Limit",
			[]grafanaTarget{
				{Expr: `sum(rate(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$instance-.*",container="openclaw"}[5m]))`, LegendFormat: "usage", RefID: "A"},
				{Expr: `sum(kube_pod_container_resource_requests{namespace="$namespace",pod=~"$instance-.*",container="openclaw",resource="cpu"})`, LegendFormat: "request", RefID: "B"},
				{Expr: `sum(kube_pod_container_resource_limits{namespace="$namespace",pod=~"$instance-.*",container="openclaw",resource="cpu"})`, LegendFormat: "limit", RefID: "C"},
			}, gp(8, 12, 0, 6)),
		timeseriesPanel(27, "CPU Throttling",
			[]grafanaTarget{
				{Expr: `sum(rate(container_cpu_cfs_throttled_seconds_total{namespace="$namespace",pod=~"$instance-.*",container="openclaw"}[5m]))`, LegendFormat: "throttled", RefID: "A"},
			}, gp(8, 12, 12, 6)),

		// --- Memory row ---
		rowPanel(202, "Memory", 14, false, nil),
		timeseriesPanel(28, "Working Set vs Limit",
			[]grafanaTarget{
				{Expr: `sum(container_memory_working_set_bytes{namespace="$namespace",pod=~"$instance-.*",container="openclaw"})`, LegendFormat: "working set", RefID: "A"},
				{Expr: `sum(kube_pod_container_resource_limits{namespace="$namespace",pod=~"$instance-.*",container="openclaw",resource="memory"})`, LegendFormat: "limit", RefID: "B"},
			}, gp(8, 12, 0, 15)),
		timeseriesPanel(29, "OOM Kills and Restarts",
			[]grafanaTarget{
				{Expr: `sum(kube_pod_container_status_restarts_total{namespace="$namespace",pod=~"$instance-.*",container="openclaw"})`, LegendFormat: "restarts", RefID: "A"},
				{Expr: `sum(kube_pod_container_status_last_terminated_reason{namespace="$namespace",pod=~"$instance-.*",container="openclaw",reason="OOMKilled"})`, LegendFormat: "OOM killed", RefID: "B"},
			}, gp(8, 12, 12, 15)),

		// --- Network row (collapsed) ---
		rowPanel(203, "Network", 23, true, []grafanaPanel{
			timeseriesPanel(30, "Network I/O",
				[]grafanaTarget{
					{Expr: `sum(rate(container_network_receive_bytes_total{namespace="$namespace",pod=~"$instance-.*"}[5m]))`, LegendFormat: "receive", RefID: "A"},
					{Expr: `sum(rate(container_network_transmit_bytes_total{namespace="$namespace",pod=~"$instance-.*"}[5m]))`, LegendFormat: "transmit", RefID: "B"},
				}, gp(8, 24, 0, 24)),
		}),

		// --- Storage row (collapsed) ---
		rowPanel(204, "Storage", 32, true, []grafanaPanel{
			timeseriesPanel(31, "PVC Usage",
				[]grafanaTarget{
					{Expr: `kubelet_volume_stats_used_bytes{namespace="$namespace",persistentvolumeclaim=~"data-$instance.*"}`, LegendFormat: "used", RefID: "A"},
					{Expr: `kubelet_volume_stats_capacity_bytes{namespace="$namespace",persistentvolumeclaim=~"data-$instance.*"}`, LegendFormat: "capacity", RefID: "B"},
				}, gp(8, 12, 0, 33)),
			gaugePanel(32, "PVC Usage %",
				`kubelet_volume_stats_used_bytes{namespace="$namespace",persistentvolumeclaim=~"data-$instance.*"} / kubelet_volume_stats_capacity_bytes{namespace="$namespace",persistentvolumeclaim=~"data-$instance.*"}`,
				gp(8, 12, 12, 33)),
		}),

		// --- Pod Health row (collapsed) ---
		rowPanel(205, "Pod Health", 41, true, []grafanaPanel{
			timeseriesPanel(33, "Container Restarts",
				[]grafanaTarget{
					{Expr: `sum(kube_pod_container_status_restarts_total{namespace="$namespace",pod=~"$instance-.*"}) by (container)`, LegendFormat: "{{ container }}", RefID: "A"},
				}, gp(8, 12, 0, 42)),
			timeseriesPanel(34, "Instance Reconciliation",
				[]grafanaTarget{
					{Expr: `sum(rate(openclaw_reconcile_total{namespace="$namespace",instance="$instance"}[5m])) by (result)`, LegendFormat: "{{ result }}", RefID: "A"},
				}, gp(8, 12, 12, 42)),
		}),
	}
	return panels
}
