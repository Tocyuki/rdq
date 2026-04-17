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
