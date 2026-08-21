package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/iszy/geo-debug-server/internal/store"
)

const maxSchemeRequestBody = 1024 * 1024

func (s *Server) handleSchemes(w http.ResponseWriter, r *http.Request, relative string) {
	parts := splitPath(relative)
	switch {
	case len(parts) == 1:
		s.handleSchemeCollection(w, r)
	case len(parts) == 2 && r.Method == http.MethodDelete:
		if err := s.store.DeleteScheme(r.Context(), parts[1]); err != nil {
			s.writeSchemeStoreError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", noStoreCache)
		w.WriteHeader(http.StatusNoContent)
	case len(parts) == 3 && strings.EqualFold(parts[2], "default") && r.Method == http.MethodPut:
		if err := s.store.SetDefaultScheme(r.Context(), parts[1]); err != nil {
			s.writeSchemeStoreError(w, r, err)
			return
		}
		scheme, err := s.store.Scheme(r.Context(), parts[1])
		if err != nil {
			s.writeSchemeStoreError(w, r, err)
			return
		}
		s.writeAGSJSON(w, r, http.StatusOK, scheme, false)
	case len(parts) == 2:
		s.writeSchemeMethodNotAllowed(w, r, "DELETE, OPTIONS")
	case len(parts) == 3 && strings.EqualFold(parts[2], "default"):
		s.writeSchemeMethodNotAllowed(w, r, "PUT, OPTIONS")
	default:
		s.writeAGSError(w, r, http.StatusNotFound, "unknown tile scheme management path", false)
	}
}

func (s *Server) handleSchemeCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		schemes, err := s.store.Schemes(r.Context())
		if err != nil {
			s.writeSchemeStoreError(w, r, err)
			return
		}
		s.writeAGSJSON(w, r, http.StatusOK, schemes, false)
	case http.MethodPost:
		scheme, err := decodeSchemeRequest(w, r)
		if err != nil {
			s.writeAGSError(w, r, http.StatusBadRequest, err.Error(), false)
			return
		}
		if err := s.store.CreateScheme(r.Context(), scheme); err != nil {
			s.writeSchemeStoreError(w, r, err)
			return
		}
		created, err := s.store.Scheme(r.Context(), scheme.ID)
		if err != nil {
			s.writeSchemeStoreError(w, r, err)
			return
		}
		w.Header().Set("Location", s.basePath+"/schemes/"+url.PathEscape(created.ID))
		s.writeAGSJSON(w, r, http.StatusCreated, created, false)
	default:
		s.writeSchemeMethodNotAllowed(w, r, "GET, HEAD, POST, OPTIONS")
	}
}

func decodeSchemeRequest(w http.ResponseWriter, r *http.Request) (store.Scheme, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSchemeRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var scheme store.Scheme
	if err := decoder.Decode(&scheme); err != nil {
		return store.Scheme{}, fmt.Errorf("decode tile scheme: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return store.Scheme{}, errors.New("decode tile scheme: multiple JSON values are not allowed")
		}
		return store.Scheme{}, fmt.Errorf("decode tile scheme: %w", err)
	}
	return scheme, nil
}

func (s *Server) writeSchemeStoreError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrInvalidScheme):
		s.writeAGSError(w, r, http.StatusBadRequest, err.Error(), false)
	case errors.Is(err, store.ErrSchemeExists):
		s.writeAGSError(w, r, http.StatusConflict, err.Error(), false)
	case errors.Is(err, store.ErrSchemeNotFound):
		s.writeAGSError(w, r, http.StatusNotFound, "unknown tile scheme", false)
	default:
		s.writeAGSError(w, r, http.StatusInternalServerError, err.Error(), false)
	}
}

func (s *Server) writeSchemeMethodNotAllowed(w http.ResponseWriter, r *http.Request, allow string) {
	w.Header().Set("Allow", allow)
	s.writeAGSError(w, r, http.StatusMethodNotAllowed, "method is not supported for this tile scheme path", false)
}
