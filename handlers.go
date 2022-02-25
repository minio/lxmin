// Copyright (c) 2015-2021 MinIO, Inc.
//
// This project is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"time"

	"github.com/gorilla/mux"
	"github.com/minio/minio-go/v7"
)

// ResponseType represents a valid LXD response type
type ResponseType string

// LXD response types
const (
	SyncResponse  ResponseType = "sync"
	AsyncResponse ResponseType = "async"
	ErrorResponse ResponseType = "error"
)

// Response represents an API response
type Response interface {
	Render(w http.ResponseWriter) error
	String() string
}

func writeErrorResponse(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusBadRequest)
	errResp := errorResponse{
		Code:  http.StatusBadRequest,
		Error: fmt.Sprintf("%v", err),
		Type:  ErrorResponse,
	}
	json.NewEncoder(w).Encode(&errResp)
}

func writeSuccessResponse(w http.ResponseWriter, data interface{}, sync bool) {
	sresp := &successResponse{
		Code:     http.StatusOK,
		Metadata: data,
		Type: func() ResponseType {
			if sync {
				return SyncResponse
			}
			return AsyncResponse
		}(),
	}
	sresp.Render(w)
}

// NotFound returns a not found response (404) with the given error.
func NotFound(err error) *errorResponse {
	message := "not found"
	if err != nil {
		message = err.Error()
	}

	return &errorResponse{
		Code:  http.StatusNotFound,
		Error: message,
		Type:  ErrorResponse,
	}
}

type errorResponse struct {
	Code  int          `json:"code"`
	Error string       `json:"error"`
	Type  ResponseType `json:"type"`
}

func (e *errorResponse) Render(w http.ResponseWriter) {
	if e == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(e.Code)
	json.NewEncoder(w).Encode(e)
}

type successResponse struct {
	Metadata interface{}  `json:"metadata"`
	Status   string       `json:"status"`
	Code     int          `json:"status_code"`
	Type     ResponseType `json:"type"`
}

func (s *successResponse) Render(w http.ResponseWriter) {
	if s == nil {
		return
	}

	s.Status = "Success"

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(s.Code)
	json.NewEncoder(w).Encode(s)
}

type backupInfo struct {
	Name       string            `json:"name"`
	Created    time.Time         `json:"created"`
	Size       int64             `json:"size"`
	Optimized  bool              `json:"optimized"`
	Compressed bool              `json:"compressed"`
	Tags       map[string]string `json:"tags,omitempty"`
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instance := vars["name"]
	backup := vars["backup"]

	if instance == "" {
		writeErrorResponse(w, errors.New("instance name cannot be empty"))
		return
	}

	if backup == "" {
		writeErrorResponse(w, errors.New("backup name cannot be empty"))
		return
	}

	opts := minio.GetObjectTaggingOptions{}
	tags, err := globalS3Clnt.GetObjectTagging(context.Background(), globalBucket, path.Join(instance, backup), opts)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	sopts := minio.StatObjectOptions{}
	obj, err := globalS3Clnt.StatObject(context.Background(), globalBucket, path.Join(instance, backup), sopts)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	_, ok := obj.UserMetadata["Optimized"]
	optimized := ok
	_, ok = obj.UserMetadata["Compressed"]
	compressed := ok

	info := backupInfo{
		Name:       backup,
		Created:    obj.LastModified,
		Size:       obj.Size,
		Optimized:  optimized,
		Compressed: compressed,
		Tags:       tags.ToMap(),
	}

	writeSuccessResponse(w, info, true)
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instance := vars["name"]

	if instance == "" {
		writeErrorResponse(w, errors.New("instance name cannot be empty"))
		return
	}

	var backups []backupInfo
	for obj := range globalS3Clnt.ListObjects(context.Background(), globalBucket, minio.ListObjectsOptions{
		Prefix:       instance,
		Recursive:    true,
		WithMetadata: true,
	}) {
		if obj.Err != nil {
			writeErrorResponse(w, obj.Err)
			return
		}
		_, ok := obj.UserMetadata["X-Amz-Meta-Optimized"]
		optimized := ok
		_, ok = obj.UserMetadata["X-Amz-Meta-Compressed"]
		compressed := ok
		backups = append(backups, backupInfo{
			Name:       path.Base(obj.Key),
			Created:    obj.LastModified,
			Size:       obj.Size,
			Optimized:  optimized,
			Compressed: compressed,
		})
	}

	writeSuccessResponse(w, backups, true)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	// A very simple health check.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
