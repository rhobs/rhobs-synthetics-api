package main

import (
	"reflect"
	"testing"
)

func TestArguments(t *testing.T) {
	for _, tc := range []struct {
		env         string
		extra, want []string
	}{
		{"", []string{"--port", "8081"}, []string{"./rhobs-synthetics-api", "start", "--port", "8081"}},
		{"dev", []string{"--database-engine", "etcd"}, []string{"./rhobs-synthetics-api", "start", "--database-engine", "local", "--database-engine", "etcd"}},
	} {
		if got := arguments(tc.env, tc.extra); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("arguments(%q, %v) = %v; want %v", tc.env, tc.extra, got, tc.want)
		}
	}
}
