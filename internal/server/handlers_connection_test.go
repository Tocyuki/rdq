package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Tocyuki/rdq/internal/connection"
	"github.com/Tocyuki/rdq/internal/state"
	"github.com/aws/aws-sdk-go-v2/aws"
)

func newTestConnectionHandlers() *connectionHandlers {
	c := newAWSCache()
	c.loader = func(_ context.Context, profile string) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	return newConnectionHandlers(c)
}

func TestClustersRequiresProfile(t *testing.T) {
	h := newTestConnectionHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/clusters", nil)
	rr := httptest.NewRecorder()
	h.clusters(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestClustersReturnsListFromMockedLister(t *testing.T) {
	h := newTestConnectionHandlers()
	h.listClusters = func(_ context.Context, _ aws.Config) ([]connection.ClusterInfo, error) {
		return []connection.ClusterInfo{
			{Identifier: "demo", ARN: "arn:c1", Engine: "aurora-mysql", Endpoint: "demo.rds", MasterUserSecretArn: "arn:s1"},
		}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/clusters?profile=dev", nil)
	rr := httptest.NewRecorder()
	h.clusters(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body ClustersDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(body.Clusters) != 1 || body.Clusters[0].Identifier != "demo" || body.Clusters[0].Engine != "aurora-mysql" {
		t.Errorf("unexpected clusters: %+v", body.Clusters)
	}
}

func TestClustersMapsAWSErrorToBadGateway(t *testing.T) {
	h := newTestConnectionHandlers()
	h.listClusters = func(_ context.Context, _ aws.Config) ([]connection.ClusterInfo, error) {
		return nil, errors.New("throttled")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/clusters?profile=dev", nil)
	rr := httptest.NewRecorder()
	h.clusters(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
}

func TestSecretsSuggestedPathWinsWhenNonEmpty(t *testing.T) {
	h := newTestConnectionHandlers()
	h.suggestSecretsForCluster = func(_ context.Context, _ aws.Config, cluster connection.ClusterInfo) ([]connection.SecretInfo, error) {
		if cluster.ARN != "arn:c1" {
			t.Errorf("unexpected cluster: %v", cluster)
		}
		return []connection.SecretInfo{
			{Name: "app/db", ARN: "arn:s1", Description: "master"},
		}, nil
	}
	h.listSecrets = func(_ context.Context, _ aws.Config) ([]connection.SecretInfo, error) {
		t.Fatal("should not fall back to full list when suggestions are non-empty")
		return nil, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/secrets?profile=dev&cluster=arn:c1", nil)
	rr := httptest.NewRecorder()
	h.secrets(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body SecretsDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Suggested || len(body.Secrets) != 1 || body.Secrets[0].ARN != "arn:s1" {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestSecretsFallsBackToListWhenSuggestionsEmpty(t *testing.T) {
	h := newTestConnectionHandlers()
	h.suggestSecretsForCluster = func(_ context.Context, _ aws.Config, _ connection.ClusterInfo) ([]connection.SecretInfo, error) {
		return nil, nil
	}
	h.listSecrets = func(_ context.Context, _ aws.Config) ([]connection.SecretInfo, error) {
		return []connection.SecretInfo{{Name: "other", ARN: "arn:s9"}}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/secrets?profile=dev&cluster=arn:c1", nil)
	rr := httptest.NewRecorder()
	h.secrets(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body SecretsDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Suggested {
		t.Errorf("expected suggested=false for fallback path")
	}
	if len(body.Secrets) != 1 || body.Secrets[0].ARN != "arn:s9" {
		t.Errorf("unexpected secrets: %+v", body.Secrets)
	}
}

func TestSecretsNoClusterListsRegion(t *testing.T) {
	h := newTestConnectionHandlers()
	h.suggestSecretsForCluster = func(_ context.Context, _ aws.Config, _ connection.ClusterInfo) ([]connection.SecretInfo, error) {
		t.Fatal("should not be called without a cluster")
		return nil, nil
	}
	h.listSecrets = func(_ context.Context, _ aws.Config) ([]connection.SecretInfo, error) {
		return []connection.SecretInfo{{Name: "any", ARN: "arn:sx"}}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/secrets?profile=dev", nil)
	rr := httptest.NewRecorder()
	h.secrets(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestDatabasesReturnsHistoryFromState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	t.Setenv("RDQ_STATE_FILE", statePath)

	// Seed state.json with a profile.
	st, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.Set("dev", state.ProfileState{
		Database:        "app",
		DatabaseHistory: []string{"app", "staging", "reporting"},
	})
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	h := newTestConnectionHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/databases?profile=dev", nil)
	rr := httptest.NewRecorder()
	h.databases(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body DatabasesDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// Most recent first — state.Set normalizes with Database as head.
	if len(body.History) < 1 || body.History[0] != "app" {
		t.Errorf("expected 'app' at head, got %v", body.History)
	}
}

func TestDatabasesUnknownProfileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_STATE_FILE", filepath.Join(dir, "state.json"))

	h := newTestConnectionHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/databases?profile=missing", nil)
	rr := httptest.NewRecorder()
	h.databases(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body DatabasesDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.History) != 0 {
		t.Errorf("expected empty history, got %v", body.History)
	}
}
