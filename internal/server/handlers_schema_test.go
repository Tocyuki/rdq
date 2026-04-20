package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tocyuki/rdq/internal/schema"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
)

func newTestSchemaHandlers() *schemaHandlers {
	c := newAWSCache()
	c.loader = func(_ context.Context, _ string) (aws.Config, error) {
		return aws.Config{}, nil
	}
	h := newSchemaHandlers(c)
	h.newClient = func(_ aws.Config) *rdsdata.Client { return nil }
	return h
}

func TestSchemaGetServesFromCache(t *testing.T) {
	h := newTestSchemaHandlers()
	h.loadCache = func(cluster, database string) (*schema.Snapshot, error) {
		return &schema.Snapshot{
			Cluster: cluster, Database: database, FetchedAt: time.Unix(1_700_000_000, 0),
			Columns: []schema.Column{{Schema: "public", Table: "users", Name: "id", Type: "int"}},
		}, nil
	}
	h.fetch = func(_ context.Context, _ *rdsdata.Client, _, _, _ string) (*schema.Snapshot, error) {
		t.Fatal("fetch should not be called when cache is populated")
		return nil, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/schema?cluster=arn:c&database=app", nil)
	rr := httptest.NewRecorder()
	h.get(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body SchemaDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.FromCache {
		t.Errorf("fromCache = false, want true")
	}
	if len(body.Columns) != 1 || body.Columns[0].Table != "users" {
		t.Errorf("unexpected columns: %+v", body.Columns)
	}
}

func TestSchemaGetFallsThroughToFetchOnCacheMiss(t *testing.T) {
	h := newTestSchemaHandlers()
	h.loadCache = func(_, _ string) (*schema.Snapshot, error) { return nil, nil }
	var saved bool
	h.saveCache = func(_ *schema.Snapshot) error { saved = true; return nil }
	h.fetch = func(_ context.Context, _ *rdsdata.Client, cluster, _, database string) (*schema.Snapshot, error) {
		return &schema.Snapshot{Cluster: cluster, Database: database, FetchedAt: time.Now(), Columns: []schema.Column{{Schema: "s", Table: "t", Name: "c", Type: "int"}}}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/schema?profile=dev&cluster=arn:c&secret=arn:s&database=app", nil)
	rr := httptest.NewRecorder()
	h.get(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !saved {
		t.Errorf("expected saveCache to be invoked after successful fetch")
	}
}

func TestSchemaGetRequiresProfileForFetch(t *testing.T) {
	h := newTestSchemaHandlers()
	h.loadCache = func(_, _ string) (*schema.Snapshot, error) { return nil, nil }
	req := httptest.NewRequest(http.MethodGet, "/api/schema?cluster=arn:c&database=app", nil)
	rr := httptest.NewRecorder()
	h.get(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestSchemaRefreshAlwaysFetches(t *testing.T) {
	h := newTestSchemaHandlers()
	h.loadCache = func(_, _ string) (*schema.Snapshot, error) {
		t.Fatal("refresh must not consult the cache")
		return nil, nil
	}
	h.saveCache = func(_ *schema.Snapshot) error { return nil }
	h.fetch = func(_ context.Context, _ *rdsdata.Client, cluster, _, database string) (*schema.Snapshot, error) {
		return &schema.Snapshot{Cluster: cluster, Database: database, FetchedAt: time.Now()}, nil
	}
	body := SchemaRefreshRequest{Profile: "dev", Cluster: "arn:c", Secret: "arn:s", Database: "app"}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/schema/refresh", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.refresh(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestSchemaRefreshMapsFetchErrorTo502(t *testing.T) {
	h := newTestSchemaHandlers()
	h.fetch = func(_ context.Context, _ *rdsdata.Client, _, _, _ string) (*schema.Snapshot, error) {
		return nil, errors.New("access denied")
	}
	body := SchemaRefreshRequest{Profile: "dev", Cluster: "arn:c", Secret: "arn:s", Database: "app"}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/schema/refresh", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.refresh(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
}
