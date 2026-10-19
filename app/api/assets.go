package main

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
)

const (
	maxFileBytes      = 20 << 20
	maxSVGBytes       = 2 << 20
	userQuotaBytes    = 1 << 30
	svgSecurityPolicy = "default-src 'none'; style-src 'unsafe-inline'; sandbox"
)

var maxBytesByType = map[string]int64{
	"image/png":        maxFileBytes,
	"image/jpeg":       maxFileBytes,
	"image/gif":        maxFileBytes,
	"image/webp":       maxFileBytes,
	"image/svg+xml":    maxSVGBytes,
	"video/mp4":        maxFileBytes,
	"video/webm":       maxFileBytes,
	"application/pdf":  maxFileBytes,
	"text/plain":       maxFileBytes,
	"text/markdown":    maxFileBytes,
	"text/csv":         maxFileBytes,
	"application/json": maxFileBytes,
}

func (s *server) uploadAsset(w http.ResponseWriter, r *http.Request, user User) {
	if s.blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "file storage is not configured")
		return
	}
	contentType := r.Header.Get("Content-Type")
	maxBytes, allowed := maxBytesByType[contentType]
	if !allowed {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported file type")
		return
	}
	size := r.ContentLength
	if size <= 0 {
		writeError(w, http.StatusLengthRequired, "file size is required")
		return
	}
	if size > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "file is too large")
		return
	}
	used, err := s.store.AssetsSize(r.Context(), user.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if used+size > userQuotaBytes {
		writeError(w, http.StatusInsufficientStorage, "file storage quota exceeded")
		return
	}

	asset := Asset{ID: randomToken(), UserID: user.ID, ContentType: contentType, SizeBytes: size}
	if err := s.blobs.Put(r.Context(), asset, http.MaxBytesReader(w, r.Body, size)); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.CreateAsset(r.Context(), asset); err != nil {
		s.blobs.Delete(r.Context(), asset)
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          asset.ID,
		"url":         "/_blob/" + asset.ID,
		"sizeBytes":   asset.SizeBytes,
		"contentType": asset.ContentType,
	})
}

func (s *server) serveAsset(w http.ResponseWriter, r *http.Request, user User) {
	if s.blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "file storage is not configured")
		return
	}
	asset, err := s.store.Asset(r.Context(), user.ID, r.PathValue("id"))
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	body, err := s.blobs.Get(r.Context(), asset)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer body.Close()

	h := w.Header()
	h.Set("Content-Type", asset.ContentType)
	h.Set("Content-Length", strconv.FormatInt(asset.SizeBytes, 10))
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")
	if asset.ContentType == "image/svg+xml" {
		h.Set("Content-Security-Policy", svgSecurityPolicy)
	}
	if _, err := io.Copy(w, body); err != nil {
		slog.Warn("serving file interrupted", "asset", asset.ID, "err", err)
	}
}

func (s *server) deleteAsset(w http.ResponseWriter, r *http.Request, user User) {
	if s.blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "file storage is not configured")
		return
	}
	asset := Asset{ID: r.PathValue("id"), UserID: user.ID}
	deleted, err := s.store.DeleteAsset(r.Context(), asset.UserID, asset.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if deleted {
		if err := s.blobs.Delete(r.Context(), asset); err != nil {
			slog.Warn("file left in bucket", "asset", asset.ID, "err", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
