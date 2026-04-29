package controller

import (
	"context"
	"sort"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openclawv1alpha1 "github.com/openclawrocks/openclaw-operator/api/v1alpha1"
)

func schemeWithOpenClaw(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := openclawv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFindInstancesForClusterDefaults_IgnoresWrongSingletonName(t *testing.T) {
	t.Parallel()
	s := schemeWithOpenClaw(t)
	r := &OpenClawInstanceReconciler{
		Client: fake.NewClientBuilder().WithScheme(s).Build(),
	}
	obj := &openclawv1alpha1.OpenClawClusterDefaults{
		ObjectMeta: metav1.ObjectMeta{Name: "not-cluster"},
	}
	if got := r.findInstancesForClusterDefaults(context.Background(), obj); len(got) != 0 {
		t.Fatalf("expected no requests, got %v", got)
	}
}

func TestFindInstancesForClusterDefaults_WatchNamespacesFiltersList(t *testing.T) {
	t.Parallel()
	s := schemeWithOpenClaw(t)
	instA := &openclawv1alpha1.OpenClawInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns-a"},
		Spec:       openclawv1alpha1.OpenClawInstanceSpec{},
	}
	instB := &openclawv1alpha1.OpenClawInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns-b"},
		Spec:       openclawv1alpha1.OpenClawInstanceSpec{},
	}
	r := &OpenClawInstanceReconciler{
		Client:          fake.NewClientBuilder().WithScheme(s).WithObjects(instA, instB).Build(),
		WatchNamespaces: []string{"ns-a"},
	}
	defaults := &openclawv1alpha1.OpenClawClusterDefaults{
		ObjectMeta: metav1.ObjectMeta{Name: openclawv1alpha1.ClusterDefaultsSingletonName},
	}
	got := r.findInstancesForClusterDefaults(context.Background(), defaults)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Namespace != "ns-a" || got[0].Name != "a" {
		t.Fatalf("got %+v, want ns-a/a", got[0])
	}
}

func TestFindInstancesForClusterDefaults_WatchNamespacesDedupesAcrossNamespaces(t *testing.T) {
	t.Parallel()
	s := schemeWithOpenClaw(t)
	// Same name in two namespaces is valid; both should appear once each.
	inst1 := &openclawv1alpha1.OpenClawInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "same", Namespace: "ns-a"},
		Spec:       openclawv1alpha1.OpenClawInstanceSpec{},
	}
	inst2 := &openclawv1alpha1.OpenClawInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "same", Namespace: "ns-b"},
		Spec:       openclawv1alpha1.OpenClawInstanceSpec{},
	}
	r := &OpenClawInstanceReconciler{
		Client:          fake.NewClientBuilder().WithScheme(s).WithObjects(inst1, inst2).Build(),
		WatchNamespaces: []string{"ns-a", "ns-b", "ns-a"},
	}
	defaults := &openclawv1alpha1.OpenClawClusterDefaults{
		ObjectMeta: metav1.ObjectMeta{Name: openclawv1alpha1.ClusterDefaultsSingletonName},
	}
	got := r.findInstancesForClusterDefaults(context.Background(), defaults)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	keys := []types.NamespacedName{got[0].NamespacedName, got[1].NamespacedName}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Namespace != keys[j].Namespace {
			return keys[i].Namespace < keys[j].Namespace
		}
		return keys[i].Name < keys[j].Name
	})
	want := []types.NamespacedName{{Namespace: "ns-a", Name: "same"}, {Namespace: "ns-b", Name: "same"}}
	if keys[0] != want[0] || keys[1] != want[1] {
		t.Fatalf("got %+v, want %+v", keys, want)
	}
}

func TestFindInstancesForClusterDefaults_EmptyWatchNamespacesListsAll(t *testing.T) {
	t.Parallel()
	s := schemeWithOpenClaw(t)
	inst := &openclawv1alpha1.OpenClawInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "solo", Namespace: "default"},
		Spec:       openclawv1alpha1.OpenClawInstanceSpec{},
	}
	r := &OpenClawInstanceReconciler{
		Client: fake.NewClientBuilder().WithScheme(s).WithObjects(inst).Build(),
	}
	defaults := &openclawv1alpha1.OpenClawClusterDefaults{
		ObjectMeta: metav1.ObjectMeta{Name: openclawv1alpha1.ClusterDefaultsSingletonName},
	}
	got := r.findInstancesForClusterDefaults(context.Background(), defaults)
	if len(got) != 1 || got[0].Name != "solo" || got[0].Namespace != "default" {
		t.Fatalf("got %+v", got)
	}
}
