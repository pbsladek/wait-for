package expr

import "testing"

func TestEvaluateJSON(t *testing.T) {
	body := []byte(`{"ready":true,"status":"ok","count":12,"items":[{"name":"first"}],"nested":{"value":3}}`)

	tests := []struct {
		name string
		expr string
		want bool
	}{
		{name: "truthy bool", expr: ".ready", want: true},
		{name: "string equality", expr: `.status == "ok"`, want: true},
		{name: "numeric greater", expr: ".count >= 10", want: true},
		{name: "array index", expr: `.items[0].name == "first"`, want: true},
		{name: "kubernetes style", expr: "{.status}=ok", want: true},
		{name: "not satisfied", expr: ".nested.value > 10", want: false},
		{name: "missing", expr: ".missing", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expression, err := Compile(tt.expr)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
			got, _, err := expression.EvaluateJSON(body)
			if err != nil {
				t.Fatalf("EvaluateJSON() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("EvaluateJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateJSONRejectsInvalidPath(t *testing.T) {
	expression, err := Compile("ready == true")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	_, _, err = expression.EvaluateJSON([]byte(`{"ready":true}`))
	if err == nil {
		t.Fatal("expected error")
	}
}
