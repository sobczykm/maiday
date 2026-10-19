package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
)

var collections = []string{"tasks", "habits", "notes", "folders", "labels", "days", "library", "jobs"}

const maxItemBytes = 1 << 20

func (s *server) listItems(w http.ResponseWriter, r *http.Request, user User) {
	items, err := s.store.Items(r.Context(), user.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	for _, c := range collections {
		if items[c] == nil {
			items[c] = []json.RawMessage{}
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) putItem(w http.ResponseWriter, r *http.Request, user User) {
	collection, id := r.PathValue("collection"), r.PathValue("id")
	if !slices.Contains(collections, collection) {
		writeError(w, http.StatusNotFound, "unknown collection")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxItemBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "item is larger than 1 MB")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}

	var item struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &item); err != nil || item.ID != id {
		writeError(w, http.StatusBadRequest, "body must be a JSON object with the same id as the URL")
		return
	}

	if err := s.store.PutItem(r.Context(), user.ID, collection, id, body); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteItem(w http.ResponseWriter, r *http.Request, user User) {
	collection, id := r.PathValue("collection"), r.PathValue("id")
	if !slices.Contains(collections, collection) {
		writeError(w, http.StatusNotFound, "unknown collection")
		return
	}
	if err := s.store.DeleteItem(r.Context(), user.ID, collection, id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
