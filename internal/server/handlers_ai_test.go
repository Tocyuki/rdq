package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tocyuki/rdq/internal/bedrock"
	"github.com/Tocyuki/rdq/internal/schema"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// fakeBedrock is a minimal implementation of the bedrockClient interface
// used by the AI handler tests. Each method returns the canned value unless
// the corresponding err is non-nil.
type fakeBedrock struct {
	models       []bedrock.ModelInfo
	askReply     string
	explainReply string
	listErr      error
	askErr       error
	explainErr   error
	seenSystem   string
	seenMessages []bedrock.Message
}

func (f *fakeBedrock) ListModels(_ context.Context) ([]bedrock.ModelInfo, error) {
	return f.models, f.listErr
}
func (f *fakeBedrock) Ask(_ context.Context, _, systemPrompt string, messages []bedrock.Message) (string, error) {
	f.seenSystem = systemPrompt
	f.seenMessages = messages
	return f.askReply, f.askErr
}
func (f *fakeBedrock) Explain(_ context.Context, _, systemPrompt string, messages []bedrock.Message) (string, error) {
	f.seenSystem = systemPrompt
	f.seenMessages = messages
	return f.explainReply, f.explainErr
}

func newTestAIHandlers(fake *fakeBedrock) *aiHandlers {
	c := newAWSCache()
	c.loader = func(_ context.Context, _ string) (aws.Config, error) { return aws.Config{}, nil }
	h := newAIHandlers(c)
	h.newClient = func(_ aws.Config) bedrockClient { return fake }
	h.loadSchema = func(_, _ string) (*schema.Snapshot, error) { return nil, nil }
	h.loadAictx = func(_, _ string) (string, error) { return "", nil }
	return h
}

func TestAIModelsHappyPath(t *testing.T) {
	fake := &fakeBedrock{models: []bedrock.ModelInfo{{ID: "anthropic.claude", Name: "Claude", Description: "LLM"}}}
	h := newTestAIHandlers(fake)
	req := httptest.NewRequest(http.MethodGet, "/api/ai/models?profile=dev", nil)
	rr := httptest.NewRecorder()
	h.models(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body ModelsDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Models) != 1 || body.Models[0].ID != "anthropic.claude" {
		t.Errorf("unexpected models: %+v", body.Models)
	}
}

func TestAIModelsRequiresProfile(t *testing.T) {
	h := newTestAIHandlers(&fakeBedrock{})
	req := httptest.NewRequest(http.MethodGet, "/api/ai/models", nil)
	rr := httptest.NewRecorder()
	h.models(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestAIAskPassesSystemPromptAndMessages(t *testing.T) {
	fake := &fakeBedrock{askReply: "SELECT 1;"}
	h := newTestAIHandlers(fake)
	payload := AskRequest{
		aiRequestBase: aiRequestBase{
			Profile: "dev", Cluster: "arn:c", Database: "app", ModelID: "m", Language: "Japanese",
		},
		Messages: []MessageDTO{{Role: "user", Text: "count users"}},
	}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/ask", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.ask(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body AskResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.SQL != "SELECT 1;" {
		t.Errorf("sql = %q", body.SQL)
	}
	if !strings.Contains(fake.seenSystem, "app") {
		t.Errorf("expected system prompt to include database, got %s", fake.seenSystem)
	}
	if len(fake.seenMessages) != 1 || fake.seenMessages[0].Role != bedrock.RoleUser {
		t.Errorf("unexpected messages forwarded: %+v", fake.seenMessages)
	}
}

func TestAIAskIncludesCurrentSQLInLatestUserMessage(t *testing.T) {
	fake := &fakeBedrock{askReply: "SELECT id, email FROM users WHERE active = true;"}
	h := newTestAIHandlers(fake)
	payload := AskRequest{
		aiRequestBase: aiRequestBase{
			Profile: "dev", Cluster: "arn:c", Database: "app", ModelID: "m", Language: "Japanese",
		},
		CurrentSQL: "SELECT id FROM users;",
		Messages: []MessageDTO{
			{Role: "user", Text: "users"},
			{Role: "assistant", Text: "SELECT id FROM users;"},
			{Role: "user", Text: "active users with email too"},
		},
	}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/ask", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.ask(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if len(fake.seenMessages) != 3 {
		t.Fatalf("messages = %+v", fake.seenMessages)
	}
	if fake.seenMessages[0].Text != "users" {
		t.Errorf("previous user turn changed: %q", fake.seenMessages[0].Text)
	}
	latest := fake.seenMessages[2].Text
	if !strings.Contains(latest, "Current SQL:\nSELECT id FROM users;") {
		t.Errorf("latest message missing current SQL:\n%s", latest)
	}
	if !strings.Contains(latest, "Request:\nactive users with email too") {
		t.Errorf("latest message missing request:\n%s", latest)
	}
}

func TestAIAskRequiresNonEmptyMessages(t *testing.T) {
	h := newTestAIHandlers(&fakeBedrock{})
	payload := AskRequest{aiRequestBase: aiRequestBase{Profile: "dev", Database: "app", ModelID: "m"}}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/ask", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.ask(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestAIExplain(t *testing.T) {
	fake := &fakeBedrock{explainReply: "the table is missing a column"}
	h := newTestAIHandlers(fake)
	payload := ExplainRequest{
		aiRequestBase: aiRequestBase{Profile: "dev", Database: "app", ModelID: "m"},
		SQL:           "SELECT * FROM t",
		ErrorMsg:      "column does not exist",
	}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/explain", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.explain(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body TextResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Text != "the table is missing a column" {
		t.Errorf("text = %q", body.Text)
	}
}

func TestAIReviewRequiresSQL(t *testing.T) {
	h := newTestAIHandlers(&fakeBedrock{})
	payload := ReviewRequest{aiRequestBase: aiRequestBase{Profile: "dev", Database: "app", ModelID: "m"}}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/review", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.review(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestAIAnalyzeRequiresSQLAndResultBlob(t *testing.T) {
	h := newTestAIHandlers(&fakeBedrock{})
	payload := AnalyzeRequest{
		aiRequestBase: aiRequestBase{Profile: "dev", Database: "app", ModelID: "m"},
		SQL:           "SELECT 1",
	}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/analyze", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.analyze(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (resultBlob missing)", rr.Code)
	}
}

func TestAIMapsErrorsTo502(t *testing.T) {
	fake := &fakeBedrock{askErr: errors.New("Bedrock throttled")}
	h := newTestAIHandlers(fake)
	payload := AskRequest{
		aiRequestBase: aiRequestBase{Profile: "dev", Database: "app", ModelID: "m"},
		Messages:      []MessageDTO{{Role: "user", Text: "hi"}},
	}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/ask", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.ask(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
}

func TestAIMapsDeadlineTo504(t *testing.T) {
	fake := &fakeBedrock{explainErr: context.DeadlineExceeded}
	h := newTestAIHandlers(fake)
	payload := ExplainRequest{
		aiRequestBase: aiRequestBase{Profile: "dev", Database: "app", ModelID: "m"},
	}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/explain", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.explain(rr, req)
	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", rr.Code)
	}
}

func TestAIAskInjectsAictxIntoSystemPrompt(t *testing.T) {
	fake := &fakeBedrock{askReply: "SELECT 1;"}
	h := newTestAIHandlers(fake)
	h.loadAictx = func(cluster, database string) (string, error) {
		if cluster == "arn:c" && database == "app" {
			return "active user = last_login_at within 30 days", nil
		}
		return "", nil
	}
	payload := AskRequest{
		aiRequestBase: aiRequestBase{
			Profile: "dev", Cluster: "arn:c", Database: "app", ModelID: "m",
		},
		Messages: []MessageDTO{{Role: "user", Text: "hi"}},
	}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/ask", strings.NewReader(string(buf)))
	rr := httptest.NewRecorder()
	h.ask(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(fake.seenSystem, "User-provided context:") {
		t.Errorf("expected user-context heading in system prompt:\n%s", fake.seenSystem)
	}
	if !strings.Contains(fake.seenSystem, "active user = last_login_at within 30 days") {
		t.Errorf("expected user-context body in system prompt:\n%s", fake.seenSystem)
	}
}

func TestAIAskTagsReadOnlyClassification(t *testing.T) {
	cases := map[string]struct {
		sql              string
		wantReadOnly     bool
		wantAutoRunnable bool
	}{
		"select":                 {"SELECT 1;", true, true},
		"explain":                {"EXPLAIN ANALYZE SELECT 1;", true, false},
		"explain analyze delete": {"EXPLAIN ANALYZE DELETE FROM users;", true, false},
		"insert":                 {"INSERT INTO users(id) VALUES (1);", false, false},
		"delete":                 {"DELETE FROM users WHERE id = 1;", false, false},
		"comments":               {"-- comment\nSELECT 1;", true, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fake := &fakeBedrock{askReply: tc.sql}
			h := newTestAIHandlers(fake)
			payload := AskRequest{
				aiRequestBase: aiRequestBase{Profile: "dev", Cluster: "arn:c", Database: "app", ModelID: "m"},
				Messages:      []MessageDTO{{Role: "user", Text: "x"}},
			}
			buf, _ := json.Marshal(payload)
			req := httptest.NewRequest(http.MethodPost, "/api/ai/ask", strings.NewReader(string(buf)))
			rr := httptest.NewRecorder()
			h.ask(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
			}
			var body AskResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.IsReadOnly != tc.wantReadOnly {
				t.Errorf("isReadOnly: got %v, want %v (sql=%q)", body.IsReadOnly, tc.wantReadOnly, tc.sql)
			}
			if body.AutoRunnable != tc.wantAutoRunnable {
				t.Errorf("autoRunnable: got %v, want %v (sql=%q)", body.AutoRunnable, tc.wantAutoRunnable, tc.sql)
			}
		})
	}
}
