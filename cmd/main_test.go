/*
Copyright 2026 OpenClaw.rocks

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"reflect"
	"testing"
)

func TestParseWatchNamespaces(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "whitespace only", in: "   ", want: nil},
		{name: "single", in: "team-a", want: []string{"team-a"}},
		{name: "multiple", in: "team-a,team-b,team-c", want: []string{"team-a", "team-b", "team-c"}},
		{name: "trims spaces", in: " team-a , team-b ", want: []string{"team-a", "team-b"}},
		{name: "drops empty entries", in: "team-a,,team-b,", want: []string{"team-a", "team-b"}},
		{name: "deduplicates", in: "team-a,team-b,team-a", want: []string{"team-a", "team-b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseWatchNamespaces(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseWatchNamespaces(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
