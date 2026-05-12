package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const (
	maxSessionBodyBytes       int64 = 64 << 10
	maxExecuteBodyBytes       int64 = 512 << 10
	maxSchemaRefreshBodyBytes int64 = 64 << 10
	maxFavoriteBodyBytes      int64 = 8 << 10
	maxAIBodyBytes            int64 = 512 << 10
	maxAnalyzeBodyBytes       int64 = 2 << 20
	maxAictxBodyBytes         int64 = 32 << 10
)

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, errCodeRequestTooLarge,
				fmt.Sprintf("request body must be <= %d bytes", maxBytes))
			return false
		}
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"invalid JSON body: "+err.Error())
		return false
	}

	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		writeJSONError(w, http.StatusBadRequest, errCodeBadRequest,
			"request body must contain a single JSON value")
		return false
	}
	return true
}
