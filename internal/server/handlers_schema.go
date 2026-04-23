package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/Tocyuki/rdq/internal/schema"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
)

// schemaHandlers serve information_schema introspection with on-disk caching.
// GET consults the cache first (no AWS round trip); POST /api/schema/refresh
// forces a fetch.
type schemaHandlers struct {
	awsCache *awsCache

	// DI seams for tests.
	loadCache func(cluster, database string) (*schema.Snapshot, error)
	saveCache func(*schema.Snapshot) error
	fetch     func(ctx context.Context, client *rdsdata.Client, cluster, secret, database string) (*schema.Snapshot, error)
	// newClient is a seam so we don't build a real rdsdata client in tests.
	newClient func(aws.Config) *rdsdata.Client
}

func newSchemaHandlers(cache *awsCache) *schemaHandlers {
	return &schemaHandlers{
		awsCache:  cache,
		loadCache: schema.LoadCache,
		saveCache: schema.SaveCache,
		fetch:     schema.Fetch,
		newClient: func(cfg aws.Config) *rdsdata.Client { return rdsdata.NewFromConfig(cfg) },
	}
}

const schemaFetchTimeout = 30 * time.Second

// get serves GET /api/schema. Cache hits (profile unused) return immediately
// so the SPA can show the schema while the user is typing, without waiting
// on AWS. Misses fall through to a synchronous fetch with a 30-second cap.
func (h *schemaHandlers) get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	profile := q.Get("profile")
	cluster := q.Get("cluster")
	secret := q.Get("secret")
	database := q.Get("database")
	if cluster == "" || database == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"cluster and database query parameters are required")
		return
	}

	if snap, err := h.loadCache(cluster, database); err == nil && snap != nil && len(snap.Columns) > 0 {
		writeJSON(w, snapshotToDTO(snap, true))
		return
	}

	if profile == "" || secret == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"profile and secret are required to fetch a fresh schema")
		return
	}

	snap, err := h.fetchAndSave(r.Context(), profile, cluster, secret, database)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}
	writeJSON(w, snapshotToDTO(snap, false))
}

// refresh serves POST /api/schema/refresh. It always talks to AWS even when a
// cache exists, and overwrites the cache on success.
func (h *schemaHandlers) refresh(w http.ResponseWriter, r *http.Request) {
	var req SchemaRefreshRequest
	if !decodeJSONBody(w, r, &req, maxSchemaRefreshBodyBytes) {
		return
	}
	if req.Profile == "" || req.Cluster == "" || req.Secret == "" || req.Database == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"profile, cluster, secret, and database are required")
		return
	}
	snap, err := h.fetchAndSave(r.Context(), req.Profile, req.Cluster, req.Secret, req.Database)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}
	writeJSON(w, snapshotToDTO(snap, false))
}

func (h *schemaHandlers) fetchAndSave(ctx context.Context, profile, cluster, secret, database string) (*schema.Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, schemaFetchTimeout)
	defer cancel()

	cfg, err := h.awsCache.Get(ctx, profile)
	if err != nil {
		return nil, err
	}
	client := h.newClient(cfg)
	snap, err := h.fetch(ctx, client, cluster, secret, database)
	if err != nil {
		return nil, err
	}
	// Best-effort cache write — a disk failure is not a reason to fail
	// the response.
	if err := h.saveCache(snap); err != nil {
		log.Printf("rdq gui: save schema cache: %v", err)
	}
	return snap, nil
}

func snapshotToDTO(s *schema.Snapshot, fromCache bool) SchemaDTO {
	dto := SchemaDTO{
		Cluster:   s.Cluster,
		Database:  s.Database,
		FetchedAt: s.FetchedAt.UTC().Format(time.RFC3339Nano),
		Columns:   make([]SchemaColumnDTO, 0, len(s.Columns)),
		FromCache: fromCache,
	}
	for _, c := range s.Columns {
		dto.Columns = append(dto.Columns, SchemaColumnDTO{
			Schema: c.Schema, Table: c.Table, Name: c.Name, Type: c.Type,
		})
	}
	return dto
}
