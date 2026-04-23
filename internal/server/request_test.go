package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBodyRejectsOversizePayload(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	var got payload
	body := `{"name":"` + strings.Repeat("x", 128) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(body))
	rr := httptest.NewRecorder()

	ok := decodeJSONBody(rr, req, &got, 32)
	if ok {
		t.Fatal("decodeJSONBody should reject an oversized payload")
	}
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %s", rr.Code, rr.Body.String())
	}
	var errBody ErrorDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("invalid error body: %v", err)
	}
	if errBody.Error.Code != errCodeRequestTooLarge {
		t.Fatalf("code = %s, want %s", errBody.Error.Code, errCodeRequestTooLarge)
	}
}

func TestDecodeJSONBodyRejectsTrailingJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	var got payload
	req := httptest.NewRequest(http.MethodPost, "/api/test", bytes.NewBufferString(`{"name":"ok"}{"extra":true}`))
	rr := httptest.NewRecorder()

	ok := decodeJSONBody(rr, req, &got, 1024)
	if ok {
		t.Fatal("decodeJSONBody should reject multiple JSON values")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}
