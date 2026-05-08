package server

import (
	"context"
	"net/http"
	"time"

	"github.com/Tocyuki/rdq/internal/aictx"
	"github.com/Tocyuki/rdq/internal/bedrock"
	"github.com/Tocyuki/rdq/internal/schema"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// bedrockClient is the subset of *bedrock.Client the AI handlers rely on,
// extracted as an interface so tests can swap in a fake without a live
// AWS Bedrock runtime.
type bedrockClient interface {
	ListModels(ctx context.Context) ([]bedrock.ModelInfo, error)
	Ask(ctx context.Context, modelID, systemPrompt string, messages []bedrock.Message) (string, error)
	Explain(ctx context.Context, modelID, systemPrompt string, messages []bedrock.Message) (string, error)
}

// aiHandlers group the five Bedrock-backed endpoints. A per-request bedrock
// client is built from the profile's aws.Config so profile switches do not
// require invalidating a long-lived handle.
type aiHandlers struct {
	awsCache *awsCache

	// DI seams for tests.
	newClient  func(cfg aws.Config) bedrockClient
	loadSchema func(cluster, database string) (*schema.Snapshot, error)
	loadAictx  func(cluster, database string) (string, error)
}

func newAIHandlers(cache *awsCache) *aiHandlers {
	return &aiHandlers{
		awsCache: cache,
		newClient: func(cfg aws.Config) bedrockClient {
			return bedrock.New(cfg)
		},
		loadSchema: schema.LoadCache,
		loadAictx:  aictx.LoadContent,
	}
}

// aiTimeout caps every Bedrock round trip. Ask/Explain/Review/Analyze all
// share the same limit because they hit the same Converse API.
const aiTimeout = 90 * time.Second

// models serves GET /api/ai/models. Profile is required because the model
// catalog is region-specific and Bedrock returns different results per
// account.
func (h *aiHandlers) models(w http.ResponseWriter, r *http.Request) {
	profile := r.URL.Query().Get("profile")
	if profile == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"profile query parameter is required")
		return
	}
	client, err := h.clientFor(r.Context(), profile)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	models, err := client.ListModels(ctx)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}
	dto := ModelsDTO{Models: make([]ModelInfoDTO, 0, len(models))}
	for _, m := range models {
		dto.Models = append(dto.Models, ModelInfoDTO{ID: m.ID, Name: m.Name, Description: m.Description})
	}
	writeJSON(w, dto)
}

// ask serves POST /api/ai/ask — natural-language → SQL.
func (h *aiHandlers) ask(w http.ResponseWriter, r *http.Request) {
	var req AskRequest
	if !decodeJSONBody(w, r, &req, maxAIBodyBytes) {
		return
	}
	if err := validateAIBase(req.aiRequestBase); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest, err.Error())
		return
	}
	if len(req.Messages) == 0 {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"messages must include at least one turn")
		return
	}

	client, err := h.clientFor(r.Context(), req.Profile)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}
	systemPrompt := bedrock.BuildSystemPrompt(req.Database, req.Language, h.aictxFor(req.Cluster, req.Database), h.snapshotFor(req.Cluster, req.Database))
	messages := toBedrockMessages(req.Messages)

	ctx, cancel := context.WithTimeout(r.Context(), aiTimeout)
	defer cancel()

	sql, err := client.Ask(ctx, req.ModelID, systemPrompt, messages)
	if err != nil {
		writeJSONError(w, aiStatus(err), errCodeAWSError, err.Error())
		return
	}
	writeJSON(w, AskResponse{SQL: sql})
}

// explain serves POST /api/ai/explain — analyze a SQL error.
func (h *aiHandlers) explain(w http.ResponseWriter, r *http.Request) {
	var req ExplainRequest
	if !decodeJSONBody(w, r, &req, maxAIBodyBytes) {
		return
	}
	if err := validateAIBase(req.aiRequestBase); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest, err.Error())
		return
	}
	client, err := h.clientFor(r.Context(), req.Profile)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}
	systemPrompt := bedrock.BuildErrorExplanationPrompt(req.Database, req.Language, h.aictxFor(req.Cluster, req.Database), h.snapshotFor(req.Cluster, req.Database))
	userPrompt := bedrock.BuildErrorUserPrompt(req.SQL, req.ErrorMsg)
	messages := []bedrock.Message{{Role: bedrock.RoleUser, Text: userPrompt}}

	ctx, cancel := context.WithTimeout(r.Context(), aiTimeout)
	defer cancel()
	text, err := client.Explain(ctx, req.ModelID, systemPrompt, messages)
	if err != nil {
		writeJSONError(w, aiStatus(err), errCodeAWSError, err.Error())
		return
	}
	writeJSON(w, TextResponse{Text: text})
}

// review serves POST /api/ai/review — critique the current SQL.
func (h *aiHandlers) review(w http.ResponseWriter, r *http.Request) {
	var req ReviewRequest
	if !decodeJSONBody(w, r, &req, maxAIBodyBytes) {
		return
	}
	if err := validateAIBase(req.aiRequestBase); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest, err.Error())
		return
	}
	if req.SQL == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest, "sql is required")
		return
	}
	client, err := h.clientFor(r.Context(), req.Profile)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}
	systemPrompt := bedrock.BuildReviewSystemPrompt(req.Database, req.Language, h.aictxFor(req.Cluster, req.Database), h.snapshotFor(req.Cluster, req.Database))
	userPrompt := bedrock.BuildReviewUserPrompt(req.SQL, req.Focus)
	messages := []bedrock.Message{{Role: bedrock.RoleUser, Text: userPrompt}}

	ctx, cancel := context.WithTimeout(r.Context(), aiTimeout)
	defer cancel()
	text, err := client.Explain(ctx, req.ModelID, systemPrompt, messages)
	if err != nil {
		writeJSONError(w, aiStatus(err), errCodeAWSError, err.Error())
		return
	}
	writeJSON(w, TextResponse{Text: text})
}

// analyze serves POST /api/ai/analyze — interpret a query's result blob.
func (h *aiHandlers) analyze(w http.ResponseWriter, r *http.Request) {
	var req AnalyzeRequest
	if !decodeJSONBody(w, r, &req, maxAnalyzeBodyBytes) {
		return
	}
	if err := validateAIBase(req.aiRequestBase); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest, err.Error())
		return
	}
	if req.SQL == "" || req.ResultBlob == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"sql and resultBlob are required")
		return
	}
	client, err := h.clientFor(r.Context(), req.Profile)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, errCodeAWSError, err.Error())
		return
	}
	systemPrompt := bedrock.BuildAnalysisSystemPrompt(req.Database, req.Language, h.aictxFor(req.Cluster, req.Database), h.snapshotFor(req.Cluster, req.Database))
	userPrompt := bedrock.BuildAnalysisUserPrompt(req.SQL, req.ResultBlob, req.Focus)
	messages := []bedrock.Message{{Role: bedrock.RoleUser, Text: userPrompt}}

	ctx, cancel := context.WithTimeout(r.Context(), aiTimeout)
	defer cancel()
	text, err := client.Explain(ctx, req.ModelID, systemPrompt, messages)
	if err != nil {
		writeJSONError(w, aiStatus(err), errCodeAWSError, err.Error())
		return
	}
	writeJSON(w, TextResponse{Text: text})
}

// clientFor resolves a bedrockClient for the given profile, backed by the
// cached aws.Config.
func (h *aiHandlers) clientFor(ctx context.Context, profile string) (bedrockClient, error) {
	cfg, err := h.awsCache.Get(ctx, profile)
	if err != nil {
		return nil, err
	}
	return h.newClient(cfg), nil
}

// snapshotFor returns the cached schema snapshot for (cluster, database) if
// one exists, or nil otherwise. AI prompts tolerate nil — it just means the
// model gets less grounding context.
func (h *aiHandlers) snapshotFor(cluster, database string) *schema.Snapshot {
	if cluster == "" || database == "" {
		return nil
	}
	snap, err := h.loadSchema(cluster, database)
	if err != nil {
		return nil
	}
	return snap
}

// aictxFor returns the saved user-context for (cluster, database) if one
// exists, or "" otherwise. Errors are swallowed so a corrupt context file
// never breaks the AI flow — same tolerance policy as the schema cache.
func (h *aiHandlers) aictxFor(cluster, database string) string {
	if cluster == "" || database == "" {
		return ""
	}
	content, err := h.loadAictx(cluster, database)
	if err != nil {
		return ""
	}
	return content
}

// validateAIBase enforces the minimum fields every AI endpoint needs.
func validateAIBase(b aiRequestBase) error {
	if b.Profile == "" {
		return errMsg("profile is required")
	}
	if b.Database == "" {
		return errMsg("database is required")
	}
	if b.ModelID == "" {
		return errMsg("modelId is required")
	}
	return nil
}

// errMsg wraps a string as an error without forcing callers to import errors.
func errMsg(s string) error { return &simpleErr{s: s} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }

// aiStatus maps a context.DeadlineExceeded to 504 and anything else to 502.
func aiStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if ctxErr := context.Canceled; err == ctxErr {
		return http.StatusInternalServerError
	}
	// Using string matching here is OK because the Converse/Bedrock
	// errors we have to deal with in production are all opaque service
	// errors with no sentinel to errors.Is against. The deadline case is
	// the one we actually care about.
	if err == context.DeadlineExceeded {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

// toBedrockMessages converts SPA-facing MessageDTO turns to the bedrock
// package's Message / Role types. Unknown roles default to user.
func toBedrockMessages(in []MessageDTO) []bedrock.Message {
	out := make([]bedrock.Message, 0, len(in))
	for _, m := range in {
		role := bedrock.RoleUser
		if m.Role == string(bedrock.RoleAssistant) {
			role = bedrock.RoleAssistant
		}
		out = append(out, bedrock.Message{Role: role, Text: m.Text})
	}
	return out
}
