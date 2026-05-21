package sampler

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	es "github.com/elastic/go-elasticsearch/v8"
)

func newClient(host, apiKey, username, password string, verifyCerts bool) (*es.Client, error) {
	cfg := es.Config{
		Addresses: []string{strings.TrimRight(host, "/")},
		// The client does not expose a per-request timeout directly; we set a
		// sensible default http.Transport below with DialContext timeouts via
		// the standard library defaults and rely on per-call contexts.
	}
	if apiKey != "" {
		cfg.APIKey = apiKey
	} else {
		cfg.Username = username
		cfg.Password = password
	}

	tr := http.DefaultTransport.(*http.Transport).Clone()
	if !verifyCerts {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 - opt-in via --no-verify-certs
	}
	cfg.Transport = tr

	return es.NewClient(cfg)
}

// newSourceClient creates the source Elasticsearch client (API key auth only).
func newSourceClient(cfg *Config) (*es.Client, error) {
	return newClient(cfg.SourceHost, cfg.SourceAPIKey, "", "", !cfg.NoVerifyCerts)
}

// newDestClient creates the destination client: API key if available, else basic auth.
func newDestClient(cfg *Config) (*es.Client, error) {
	if cfg.DestAPIKey != "" {
		return newClient(cfg.DestHost, cfg.DestAPIKey, "", "", !cfg.NoVerifyCerts)
	}
	return newClient(cfg.DestHost, "", cfg.DestUsername, cfg.DestPassword, !cfg.NoVerifyCerts)
}

// pingCluster pings a cluster and logs its name and version.
func pingCluster(ctx context.Context, client *es.Client, label string, log Logger, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := client.Info(client.Info.WithContext(ctx))
	if err != nil {
		log.Logf("[%s] Connection failed: %v", label, err)
		return err
	}
	defer res.Body.Close()
	if res.IsError() {
		log.Logf("[%s] Connection failed: %s", label, res.String())
		return fmt.Errorf("%s: %s", label, res.String())
	}

	var body struct {
		ClusterName string `json:"cluster_name"`
		Version     struct {
			Number string `json:"number"`
		} `json:"version"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		log.Logf("[%s] Connection failed: %v", label, err)
		return err
	}

	name := body.ClusterName
	if name == "" {
		name = "unknown"
	}
	version := body.Version.Number
	if version == "" {
		version = "unknown"
	}
	log.Logf("[%s] Connected: %s (%s)", label, name, version)
	return nil
}
