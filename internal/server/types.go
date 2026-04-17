package server

// SessionDTO is the JSON representation of the server's "current connection"
// pointer. It is returned by GET /api/session and accepted by PUT /api/session.
// All fields may be empty — the GUI is responsible for nudging the user into
// filling them in (via ConnectionDialog) before calling an endpoint that
// actually needs them.
type SessionDTO struct {
	Profile         string `json:"profile"`
	Cluster         string `json:"cluster"`
	Secret          string `json:"secret"`
	Database        string `json:"database"`
	BedrockModel    string `json:"bedrockModel"`
	BedrockLanguage string `json:"bedrockLanguage"`
}

// HealthDTO is the JSON body of /api/health.
type HealthDTO struct {
	Status string `json:"status"`
}

// ProfilesDTO is the JSON body of /api/profiles.
type ProfilesDTO struct {
	Profiles []string `json:"profiles"`
}

// ClusterInfoDTO is the JSON shape of a single Aurora cluster returned by
// /api/clusters. Engine lets the SPA pick MySQL vs PostgreSQL when
// configuring CodeMirror's SQL dialect.
type ClusterInfoDTO struct {
	Identifier          string `json:"identifier"`
	ARN                 string `json:"arn"`
	Engine              string `json:"engine"`
	Endpoint            string `json:"endpoint"`
	MasterUserSecretArn string `json:"masterUserSecretArn,omitempty"`
}

// ClustersDTO wraps the /api/clusters list.
type ClustersDTO struct {
	Clusters []ClusterInfoDTO `json:"clusters"`
}

// SecretInfoDTO is the JSON shape of a Secrets Manager secret.
type SecretInfoDTO struct {
	Name        string `json:"name"`
	ARN         string `json:"arn"`
	Description string `json:"description,omitempty"`
}

// SecretsDTO wraps the /api/secrets list. Suggested=true means the list is
// scoped to the given cluster (MasterUserSecret + tag filters); false means
// the SPA is looking at the full region listing as a fallback.
type SecretsDTO struct {
	Secrets   []SecretInfoDTO `json:"secrets"`
	Suggested bool            `json:"suggested"`
}

// DatabasesDTO wraps the /api/databases response. History is the list of DB
// names previously used for this profile (most recent first), as tracked in
// state.json.
type DatabasesDTO struct {
	History []string `json:"history"`
}

// ExecuteRequest is the JSON body of POST /api/execute. All four connection
// fields are required because the server is stateless — it does not assume
// the session store has the right values at the moment the request arrives.
type ExecuteRequest struct {
	Profile  string `json:"profile"`
	Cluster  string `json:"cluster"`
	Secret   string `json:"secret"`
	Database string `json:"database"`
	SQL      string `json:"sql"`
}

// ExecuteResponse is the JSON body returned by POST /api/execute. Rows is a
// [][]any so SELECT output preserves native Go types (int64/float64/bool/
// []byte-as-base64/null) through JSON marshaling.
type ExecuteResponse struct {
	Columns    []string `json:"columns"`
	Rows       [][]any  `json:"rows"`
	Updated    int64    `json:"updated"`
	DurationMS int64    `json:"durationMs"`
}

// SchemaColumnDTO is one row from information_schema.columns.
type SchemaColumnDTO struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Name   string `json:"name"`
	Type   string `json:"type"`
}

// SchemaDTO is the JSON body of /api/schema. FromCache=true means the
// snapshot was served from the on-disk cache without an AWS round trip.
type SchemaDTO struct {
	Cluster   string            `json:"cluster"`
	Database  string            `json:"database"`
	FetchedAt string            `json:"fetchedAt"`
	Columns   []SchemaColumnDTO `json:"columns"`
	FromCache bool              `json:"fromCache"`
}

// SchemaRefreshRequest is the JSON body of POST /api/schema/refresh.
type SchemaRefreshRequest struct {
	Profile  string `json:"profile"`
	Cluster  string `json:"cluster"`
	Secret   string `json:"secret"`
	Database string `json:"database"`
}

// HistoryEntryDTO is the JSON shape of a /api/history entry. Times are
// rendered as RFC3339Nano strings so the SPA can round-trip them through
// POST /api/history/favorite.
type HistoryEntryDTO struct {
	Profile    string `json:"profile"`
	Database   string `json:"database"`
	SQL        string `json:"sql"`
	At         string `json:"at"`
	Ok         bool   `json:"ok"`
	DurationMS int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
	Favorite   bool   `json:"favorite,omitempty"`
}

// HistoryDTO wraps the /api/history list (most recent first).
type HistoryDTO struct {
	Entries []HistoryEntryDTO `json:"entries"`
}

// FavoriteRequest is the JSON body of POST /api/history/favorite. At
// identifies the entry (history is keyed by execution timestamp).
type FavoriteRequest struct {
	At       string `json:"at"`
	Favorite bool   `json:"favorite"`
}

// ErrorDTO is the uniform shape for error responses. Code is a short, stable
// string enum for programmatic handling; Message is free-form text for
// display.
type ErrorDTO struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail is the nested object inside ErrorDTO.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
