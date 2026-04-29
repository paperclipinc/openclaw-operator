package watchnamespaces

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty", raw: "", want: nil},
		{name: "whitespace", raw: "  \n\t ", want: nil},
		{name: "single", raw: "openclaw", want: []string{"openclaw"}},
		{name: "trim", raw: " a , b ", want: []string{"a", "b"}},
		{name: "dedupe", raw: "ns1,ns1,ns2", want: []string{"ns1", "ns2"}},
		{name: "skip_empty_segments", raw: "foo,,bar", want: []string{"foo", "bar"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Parse(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestFromEnv(t *testing.T) {
	t.Run("uses env var", func(t *testing.T) {
		t.Setenv(envVar, "ns-one,ns-two")
		got := FromEnv()
		if !reflect.DeepEqual(got, []string{"ns-one", "ns-two"}) {
			t.Fatalf("FromEnv = %#v", got)
		}
	})
	t.Run("empty_env", func(t *testing.T) {
		t.Setenv(envVar, "")
		if FromEnv() != nil {
			t.Fatalf("expected nil when env is empty")
		}
	})
}
