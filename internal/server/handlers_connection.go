package server

import (
	"context"
	"net/http"
	"time"

	"github.com/Tocyuki/rdq/internal/connection"
	"github.com/Tocyuki/rdq/internal/state"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// connectionHandlers serves endpoints that let the SPA walk the
// profile→cluster→secret→database wizard without touching the TTY-based
// fuzzyfinder helpers in internal/connection.
type connectionHandlers struct {
	awsCache *awsCache

	// DI seams for tests. Default to the real connection package.
	listClusters             func(ctx context.Context, cfg aws.Config) ([]connection.ClusterInfo, error)
	listSecrets              func(ctx context.Context, cfg aws.Config) ([]connection.SecretInfo, error)
	suggestSecretsForCluster func(ctx context.Context, cfg aws.Config, cluster connection.ClusterInfo) ([]connection.SecretInfo, error)

	// loadState returns the per-profile state for /api/databases. Seam
	// for tests that don't want to touch ~/.rdq/state.json.
	loadState func() (*state.State, error)
}

func newConnectionHandlers(c *awsCache) *connectionHandlers {
	return &connectionHandlers{
		awsCache:                 c,
		listClusters:             connection.ListClusters,
		listSecrets:              connection.ListSecrets,
		suggestSecretsForCluster: connection.SuggestSecretsForCluster,
		loadState:                state.Load,
	}
}

// connectionTimeout bounds the AWS round trips taken by these endpoints. The
// SPA shows a spinner while waiting, so we prefer a short deadline over the
// SDK's defaults.
const connectionTimeout = 30 * time.Second

// clusters enumerates the Data-API-enabled Aurora clusters for the requested
// profile. The profile is taken from ?profile= so the endpoint is stateless.
func (h *connectionHandlers) clusters(w http.ResponseWriter, r *http.Request) {
	profile := r.URL.Query().Get("profile")
	if profile == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"profile query parameter is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), connectionTimeout)
	defer cancel()

	cfg, err := h.awsCache.Get(ctx, profile)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}
	clusters, err := h.listClusters(ctx, cfg)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}

	dto := ClustersDTO{Clusters: make([]ClusterInfoDTO, 0, len(clusters))}
	for _, c := range clusters {
		dto.Clusters = append(dto.Clusters, ClusterInfoDTO{
			Identifier:          c.Identifier,
			ARN:                 c.ARN,
			Engine:              c.Engine,
			Endpoint:            c.Endpoint,
			MasterUserSecretArn: c.MasterUserSecretArn,
		})
	}
	writeJSON(w, dto)
}

// secrets returns Secrets Manager secrets usable with the given cluster. When
// cluster is omitted the handler falls back to listing every secret in the
// region.
func (h *connectionHandlers) secrets(w http.ResponseWriter, r *http.Request) {
	profile := r.URL.Query().Get("profile")
	if profile == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"profile query parameter is required")
		return
	}
	clusterARN := r.URL.Query().Get("cluster")

	ctx, cancel := context.WithTimeout(r.Context(), connectionTimeout)
	defer cancel()

	cfg, err := h.awsCache.Get(ctx, profile)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}

	var (
		secrets   []connection.SecretInfo
		suggested bool
	)
	if clusterARN != "" {
		suggestions, suggErr := h.suggestSecretsForCluster(ctx, cfg, connection.ClusterInfo{ARN: clusterARN})
		if suggErr == nil && len(suggestions) > 0 {
			secrets = suggestions
			suggested = true
		}
	}
	if !suggested {
		all, listErr := h.listSecrets(ctx, cfg)
		if listErr != nil {
			writeJSONError(w, http.StatusBadGateway, errCodeAWSError, listErr.Error())
			return
		}
		secrets = all
	}

	dto := SecretsDTO{
		Secrets:   make([]SecretInfoDTO, 0, len(secrets)),
		Suggested: suggested,
	}
	for _, s := range secrets {
		dto.Secrets = append(dto.Secrets, SecretInfoDTO{
			Name:        s.Name,
			ARN:         s.ARN,
			Description: s.Description,
		})
	}
	writeJSON(w, dto)
}

// databases returns the per-profile database name history from state.json.
// We deliberately do not query AWS for a live database list because the Data
// API has no such primitive — the history is the best approximation of "DBs
// the user has opened on this cluster".
func (h *connectionHandlers) databases(w http.ResponseWriter, r *http.Request) {
	profile := r.URL.Query().Get("profile")
	if profile == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"profile query parameter is required")
		return
	}

	st, err := h.loadState()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, errCodeInternal,
			"load state.json: "+err.Error())
		return
	}
	ps := st.Get(profile)
	history := ps.DatabaseHistory
	if history == nil {
		history = []string{}
	}
	writeJSON(w, DatabasesDTO{History: history})
}
