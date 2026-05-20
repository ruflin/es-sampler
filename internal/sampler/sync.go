package sampler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	es "github.com/elastic/go-elasticsearch/v8"
)

// document is the minimal representation of an Elasticsearch hit that flows
// through search -> transform -> upload.
type document struct {
	Index  string         `json:"_index,omitempty"`
	ID     string         `json:"_id,omitempty"`
	Source map[string]any `json:"_source,omitempty"`
	Score  *float64       `json:"_score,omitempty"`
}

func nowMillis() int64 { return time.Now().UTC().UnixMilli() }

// search runs the per-cycle search and returns hit documents. Transient
// failures are logged and surfaced as an empty slice so the main loop keeps
// cycling.
func search(
	ctx context.Context,
	client *es.Client,
	cfg *Config,
	cycleNumber int,
	log Logger,
) []*document {
	gte := fmt.Sprintf("now-%ds", int64(cfg.Lookback.Seconds()))
	baseQuery := map[string]any{
		"bool": map[string]any{
			"filter": []any{
				map[string]any{"range": map[string]any{"@timestamp": map[string]string{
					"gte": gte,
					"lte": "now",
				}}},
			},
		},
	}

	var seed int64
	if cfg.RandomSeed != nil {
		seed = *cfg.RandomSeed
	} else {
		base := cfg.RunSeed
		if base == 0 {
			base = nowMillis()
		}
		seed = base + int64(cycleNumber)
	}
	body := map[string]any{
		"query": map[string]any{
			"function_score": map[string]any{
				"query": baseQuery,
				"functions": []any{
					map[string]any{"random_score": map[string]any{"seed": seed}},
				},
				"boost_mode": "replace",
			},
		},
		"size": cfg.Size,
	}

	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		log.Logf("Search failed: %v", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	res, err := client.Search(
		client.Search.WithContext(ctx),
		client.Search.WithIndex(cfg.IndexPattern),
		client.Search.WithBody(buf),
	)
	if err != nil {
		log.Logf("Search failed: %v", err)
		return nil
	}
	defer res.Body.Close()
	if res.IsError() {
		log.Logf("Search failed: %s", res.String())
		return nil
	}

	var decoded struct {
		Hits struct {
			Hits []struct {
				Index  string          `json:"_index"`
				ID     string          `json:"_id"`
				Source json.RawMessage `json:"_source"`
				Score  *float64        `json:"_score"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		log.Logf("Search failed: %v", err)
		return nil
	}

	docs := make([]*document, 0, len(decoded.Hits.Hits))
	for _, h := range decoded.Hits.Hits {
		var src map[string]any
		if len(h.Source) > 0 {
			if err := json.Unmarshal(h.Source, &src); err != nil {
				src = map[string]any{}
			}
		} else {
			src = map[string]any{}
		}
		docs = append(docs, &document{
			Index:  h.Index,
			ID:     h.ID,
			Source: src,
			Score:  h.Score,
		})
	}
	return docs
}

// transform mutates docs in place: strip data_stream fields, drop any
// cross-cluster `<alias>:` prefix from the source index, and override the
// destination index when TargetIndex is set.
func transform(docs []*document, cfg *Config) {
	for _, doc := range docs {
		stripDataStreamFields(doc)
		if cfg.TargetIndex != "" {
			doc.Index = cfg.TargetIndex
			continue
		}
		if i := strings.IndexByte(doc.Index, ':'); i >= 0 {
			doc.Index = doc.Index[i+1:]
		}
	}
}

func stripDataStreamFields(doc *document) {
	if doc == nil || doc.Source == nil {
		return
	}
	for k := range doc.Source {
		if strings.HasPrefix(k, "data_stream.") {
			delete(doc.Source, k)
		}
	}
}

// backingIndexToStreamName maps a `.ds-<stream>-<date>-<gen>` backing index
// to `<stream>`. When the index came from a cross-cluster search hit it is
// prefixed with `<cluster_alias>:` — strip that prefix so the destination
// cluster receives a local name it can write to (without it, the destination
// rejects the write with "Cross-cluster calls are not supported").
func backingIndexToStreamName(index string) string {
	if i := strings.IndexByte(index, ':'); i >= 0 {
		index = index[i+1:]
	}
	if !strings.HasPrefix(index, ".ds-") {
		return index
	}
	withoutPrefix := strings.TrimPrefix(index, ".ds-")
	parts := strings.Split(withoutPrefix, "-")
	if len(parts) >= 3 {
		return strings.Join(parts[:len(parts)-2], "-")
	}
	return index
}

func ensureDataStreamExists(ctx context.Context, client *es.Client, name string, log Logger) bool {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	res, err := client.Indices.GetDataStream(
		client.Indices.GetDataStream.WithContext(ctx),
		client.Indices.GetDataStream.WithName(name),
	)
	if err != nil {
		log.Logf("Failed to get data stream %s: %v", name, err)
		return false
	}
	defer res.Body.Close()

	if !res.IsError() {
		io.Copy(io.Discard, res.Body)
		return true
	}
	if res.StatusCode != 404 {
		log.Logf("Failed to get data stream %s: %s", name, res.String())
		return false
	}

	createCtx, cancel2 := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel2()
	createRes, err := client.Indices.CreateDataStream(
		name,
		client.Indices.CreateDataStream.WithContext(createCtx),
	)
	if err != nil {
		log.Logf("Failed to create data stream %s: %v", name, err)
		return false
	}
	defer createRes.Body.Close()
	if createRes.IsError() {
		log.Logf("Failed to create data stream %s: %s", name, createRes.String())
		return false
	}
	log.Logf("Created data stream: %s", name)
	return true
}

func uploadDocumentsToStream(
	ctx context.Context,
	client *es.Client,
	docs []*document,
	target string,
	batchSize int,
	log Logger,
) int {
	if len(docs) == 0 {
		return 0
	}
	if !ensureDataStreamExists(ctx, client, target, log) {
		return 0
	}

	total := 0
	for start := 0; start < len(docs); start += batchSize {
		end := start + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		batch := docs[start:end]

		var buf bytes.Buffer
		for _, doc := range batch {
			meta := map[string]any{"create": map[string]any{"_index": target}}
			if err := json.NewEncoder(&buf).Encode(meta); err != nil {
				log.Logf("Bulk request failed: %v", err)
				return total
			}
			src := doc.Source
			if src == nil {
				src = map[string]any{}
			}
			if err := json.NewEncoder(&buf).Encode(src); err != nil {
				log.Logf("Bulk request failed: %v", err)
				return total
			}
		}

		bulkCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		res, err := client.Bulk(
			bytes.NewReader(buf.Bytes()),
			client.Bulk.WithContext(bulkCtx),
			client.Bulk.WithRefresh("false"),
		)
		cancel()
		if err != nil {
			log.Logf("Bulk request failed: %v", err)
			continue
		}

		var parsed struct {
			Errors bool `json:"errors"`
			Items  []struct {
				Create *struct {
					Index  string          `json:"_index"`
					ID     string          `json:"_id"`
					Status int             `json:"status"`
					Error  json.RawMessage `json:"error"`
				} `json:"create"`
			} `json:"items"`
		}
		if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
			log.Logf("Bulk request failed: %v", err)
			res.Body.Close()
			continue
		}
		res.Body.Close()

		if parsed.Errors {
			for _, item := range parsed.Items {
				if item.Create == nil {
					continue
				}
				if len(item.Create.Error) > 0 {
					reason := extractReason(item.Create.Error)
					id := item.Create.ID
					if id == "" {
						id = "auto-generated"
					}
					log.Logf("Bulk create error [%s] (ES _id: %s): %s", item.Create.Index, id, reason)
					continue
				}
				total++
			}
		} else {
			total += len(batch)
		}
	}
	return total
}

func extractReason(raw json.RawMessage) string {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if r, ok := obj["reason"].(string); ok && r != "" {
			return r
		}
	}
	return string(raw)
}

// runSyncCycle runs one sync cycle and returns the number of documents
// uploaded. Search/upload failures are logged inside and surfaced as a lower
// count rather than an error so the caller's loop keeps cycling.
func runSyncCycle(
	ctx context.Context,
	sourceClient, destClient *es.Client,
	cfg *Config,
	cycleNumber int,
	log Logger,
) int {
	docs := search(ctx, sourceClient, cfg, cycleNumber, log)
	if len(docs) == 0 {
		return 0
	}

	transform(docs, cfg)

	if cfg.TargetIndex != "" {
		return uploadDocumentsToStream(ctx, destClient, docs, cfg.TargetIndex, cfg.BatchSize, log)
	}

	byStream := map[string][]*document{}
	for _, doc := range docs {
		name := backingIndexToStreamName(doc.Index)
		byStream[name] = append(byStream[name], doc)
	}
	total := 0
	for name, group := range byStream {
		total += uploadDocumentsToStream(ctx, destClient, group, name, cfg.BatchSize, log)
	}
	return total
}

// Run is the main loop: connect, then run cycles until the context is
// cancelled.
func Run(ctx context.Context, cfg *Config, log Logger) error {
	if cfg.RandomSeed == nil {
		cfg.RunSeed = nowMillis()
	}

	sourceClient, err := newSourceClient(cfg)
	if err != nil {
		return fmt.Errorf("source client: %w", err)
	}
	destClient, err := newDestClient(cfg)
	if err != nil {
		return fmt.Errorf("dest client: %w", err)
	}

	if err := pingCluster(ctx, sourceClient, "source", log); err != nil {
		return err
	}
	if err := pingCluster(ctx, destClient, "dest", log); err != nil {
		return err
	}

	cycle := 0
	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log("Sync stopped.")
				return nil
			}
			return err
		}
		cycle++

		pushed := runSyncCycle(ctx, sourceClient, destClient, cfg, cycle, log)
		log.Logf("Cycle %d: pushed %d documents", cycle, pushed)

		if err := ctx.Err(); err != nil {
			log("Sync stopped.")
			return nil
		}
		select {
		case <-ctx.Done():
			log("Sync stopped.")
			return nil
		case <-time.After(cfg.Interval):
		}
	}
}
