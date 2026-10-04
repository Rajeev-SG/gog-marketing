// Package provider contains the non-secret hosted v1 provider inventory and
// safe, read-only preflight checks for the provider resources referenced by
// the hosted v1 epic.
package provider

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
)

//go:embed config.json
var configData []byte

// Config is the central, non-secret inventory of provider resources.
type Config struct {
	SchemaVersion int        `json:"schema_version"`
	Cloudflare    Cloudflare `json:"cloudflare"`
	Clerk         Clerk      `json:"clerk"`
	GCP           GCP        `json:"gcp"`
	Environment   []Binding  `json:"environment"`
	Secrets       []Binding  `json:"secrets"`
}

// Cloudflare names the control-plane resources.
type Cloudflare struct {
	Worker      string `json:"worker"`
	D1Database  string `json:"d1_database"`
	KVNamespace string `json:"kv_namespace"`
}

// Clerk names the hosted identity application.
type Clerk struct {
	Application string `json:"application"`
}

// GCP names the data-plane resources.
type GCP struct {
	Project          string           `json:"project"`
	ArtifactRegistry ArtifactRegistry `json:"artifact_registry"`
	CloudRun         CloudRun         `json:"cloud_run"`
}

// ArtifactRegistry names the Cloud Run image repository.
type ArtifactRegistry struct {
	Repository string `json:"repository"`
	Location   string `json:"location"`
}

// CloudRun names the private execution service and its identity.
type CloudRun struct {
	Service        string `json:"service"`
	Region         string `json:"region"`
	RunnerIdentity string `json:"runner_identity"`
}

// Binding describes the name and target surface for a provider setting.
type Binding struct {
	Name    string `json:"name"`
	Surface string `json:"surface"`
	Secret  bool   `json:"secret"`
	Purpose string `json:"purpose"`
}

var errUnsupportedSchema = errors.New("unsupported provider inventory schema version")

// Load returns the embedded provider inventory. It intentionally contains no
// secret values.
func Load() (Config, error) {
	var config Config
	if err := json.Unmarshal(configData, &config); err != nil {
		return Config{}, fmt.Errorf("decode provider inventory: %w", err)
	}

	if config.SchemaVersion != 1 {
		return Config{}, fmt.Errorf("%w: %d", errUnsupportedSchema, config.SchemaVersion)
	}

	return config, nil
}
