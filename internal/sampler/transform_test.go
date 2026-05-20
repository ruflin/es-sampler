package sampler

import (
	"testing"
	"time"
)

func baseConfig() *Config {
	return &Config{
		SourceHost:   "http://src:9200",
		SourceAPIKey: "k",
		DestHost:     "http://dst:9200",
		DestUsername: "elastic",
		DestPassword: "changeme",
		IndexPattern: "logs*",
		Size:         100,
		Interval:     time.Second,
		Lookback:     24 * time.Hour,
		BatchSize:    100,
	}
}

func TestTransform_StripsDataStreamFields(t *testing.T) {
	docs := []*document{{
		Index: "logs-foo",
		ID:    "1",
		Source: map[string]any{
			"@timestamp":            "2024-01-01T00:00:00Z",
			"data_stream.type":      "logs",
			"data_stream.dataset":   "foo",
			"data_stream.namespace": "default",
			"message":               "hello",
		},
	}}
	transform(docs, baseConfig())
	src := docs[0].Source
	for _, k := range []string{"data_stream.type", "data_stream.dataset", "data_stream.namespace"} {
		if _, ok := src[k]; ok {
			t.Fatalf("expected %q to be stripped", k)
		}
	}
	if src["message"] != "hello" {
		t.Fatalf("message: %v", src["message"])
	}
}

func TestTransform_TargetIndexOverride(t *testing.T) {
	docs := []*document{{Index: "logs-original", ID: "1", Source: map[string]any{}}}
	cfg := baseConfig()
	cfg.TargetIndex = "logs-new"
	transform(docs, cfg)
	if docs[0].Index != "logs-new" {
		t.Fatalf("index: %q", docs[0].Index)
	}
}

func TestTransform_LeavesTimestampUnchanged(t *testing.T) {
	docs := []*document{{
		Index:  "logs-foo",
		ID:     "1",
		Source: map[string]any{"@timestamp": "2020-01-01T00:00:00Z"},
	}}
	transform(docs, baseConfig())
	if docs[0].Source["@timestamp"] != "2020-01-01T00:00:00Z" {
		t.Fatalf("timestamp mutated: %v", docs[0].Source["@timestamp"])
	}
}

func TestTransform_StripsCrossClusterPrefix(t *testing.T) {
	docs := []*document{
		{Index: "monitor:.ds-logs-elastic_agent.filebeat-default-2026.05.07-008246", ID: "1", Source: map[string]any{}},
		{Index: "monitor:logs-foo", ID: "2", Source: map[string]any{}},
	}
	transform(docs, baseConfig())
	if docs[0].Index != ".ds-logs-elastic_agent.filebeat-default-2026.05.07-008246" {
		t.Fatalf("backing index: %q", docs[0].Index)
	}
	if docs[1].Index != "logs-foo" {
		t.Fatalf("plain index: %q", docs[1].Index)
	}
}

func TestTransform_TargetIndexBeatsCrossClusterPrefix(t *testing.T) {
	docs := []*document{{Index: "monitor:logs-foo", ID: "1", Source: map[string]any{}}}
	cfg := baseConfig()
	cfg.TargetIndex = "logs-new"
	transform(docs, cfg)
	if docs[0].Index != "logs-new" {
		t.Fatalf("index: %q", docs[0].Index)
	}
}

func TestBackingIndexToStreamName_CrossCluster(t *testing.T) {
	cases := map[string]string{
		"monitor:.ds-logs-elastic_agent.filebeat-default-2026.05.07-008246": "logs-elastic_agent.filebeat-default",
		"monitor:logs-foo":           "logs-foo",
		".ds-logs-foo-2026.01.01-01": "logs-foo",
		"logs-foo":                   "logs-foo",
	}
	for in, want := range cases {
		if got := backingIndexToStreamName(in); got != want {
			t.Fatalf("backingIndexToStreamName(%q) = %q, want %q", in, got, want)
		}
	}
}
