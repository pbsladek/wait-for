package condition

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestKubernetesConditionNamedCondition(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), podObject("True"))
	cond := NewKubernetes("pod/myapp")
	cond.Getter = NewDynamicKubernetesGetterWithClient(client)
	cond.Condition = "Ready"

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("Satisfied = false, err = %v, detail = %q", result.Err, result.Detail)
	}
}

func TestKubernetesConditionJSONPath(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), podObject("False"))
	cond := NewKubernetes("pod/myapp")
	cond.Getter = NewDynamicKubernetesGetterWithClient(client)
	cond.JSONPath = "{.status.phase}=Running"

	result := cond.Check(t.Context())
	if result.Status != CheckSatisfied {
		t.Fatalf("Satisfied = false, err = %v, detail = %q", result.Err, result.Detail)
	}
}

func TestKubernetesConditionNotReady(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), podObject("False"))
	cond := NewKubernetes("pod/myapp")
	cond.Getter = NewDynamicKubernetesGetterWithClient(client)
	cond.Condition = "Ready"

	result := cond.Check(t.Context())
	if result.Status == CheckSatisfied {
		t.Fatal("Satisfied = true, want false")
	}
}

func podObject(ready string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      "myapp",
			"namespace": "default",
		},
		"status": map[string]any{
			"phase": "Running",
			"conditions": []any{
				map[string]any{"type": "Ready", "status": ready},
			},
		},
	}}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
	return obj
}
