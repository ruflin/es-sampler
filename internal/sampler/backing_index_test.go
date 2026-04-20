package sampler

import "testing"

func TestBackingIndexToStreamName(t *testing.T) {
	cases := []struct {
		name, index, want string
	}{
		{"apache", ".ds-logs-apache-default-2024.01.02-000001", "logs-apache-default"},
		{"nginx", ".ds-logs-nginx-2024.01.02-000001", "logs-nginx"},
		{"metrics", ".ds-metrics-2024.01.02-000001", "metrics"},
		{"not_backing", "logs-stream", "logs-stream"},
		{"empty", "", ""},
		{"too_short", ".ds-short", ".ds-short"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := backingIndexToStreamName(tc.index); got != tc.want {
				t.Fatalf("backingIndexToStreamName(%q) = %q, want %q", tc.index, got, tc.want)
			}
		})
	}
}
