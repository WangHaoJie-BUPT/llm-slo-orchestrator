package router

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const maxControlBodyBytes = 1024

func WithControlAPI(handler *Handler, token string) http.Handler {
	if strings.TrimSpace(token) == "" {
		return handler
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/strategy", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		if request.Method != http.MethodPost {
			writeJSONError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !validBearer(request.Header.Get("Authorization"), token) {
			writeJSONError(writer, http.StatusUnauthorized, "invalid control token")
			return
		}
		var payload struct {
			Strategy string `json:"strategy"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxControlBodyBytes))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			writeJSONError(writer, http.StatusBadRequest, "invalid strategy request")
			return
		}
		if err := ensureJSONEnd(decoder); err != nil {
			writeJSONError(writer, http.StatusBadRequest, "invalid strategy request")
			return
		}
		if err := handler.SetStrategy(payload.Strategy); err != nil {
			writeJSONError(writer, http.StatusBadRequest, "unsupported routing strategy")
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(handler.Status())
	})
	mux.Handle("/", handler)
	return mux
}

func validBearer(header, token string) bool {
	value, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || value == "" {
		return false
	}
	expected := sha256.Sum256([]byte(token))
	actual := sha256.Sum256([]byte(value))
	return subtle.ConstantTimeCompare(expected[:], actual[:]) == 1
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}
