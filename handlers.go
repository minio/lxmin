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

func writeErrorResponse(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusBadRequest)
	errResp := errorResponse{
		Code:  http.StatusBadRequest,
		Error: fmt.Sprintf("%v", err),
		Type:  "error",
	}
	json.NewEncoder(w).Encode(&errResp)
}

func writeSuccessResponse(w http.ResponseWriter, data interface{}, sync bool) {
	w.WriteHeader(http.StatusOK)
	successResp := successResponse{
		Metadata:   data,
		Status:     "Success",
		StatusCode: http.StatusOK,
		Type: func() string {
			if sync {
				return "sync"
			}
			return "async"
		}(),
	}
	json.NewEncoder(w).Encode(&successResp)
}

type errorResponse struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
	Type  string `json:"type"`
}

type successResponse struct {
	Metadata   interface{} `json:"metadata"`
	Status     string      `json:"status"`
	StatusCode int         `json:"status_code"`
	Type       string      `json:"type"`
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

	_, ok := obj.UserMetadata["X-Amz-Meta-Optimized"]
	optimized := ok
	_, ok = obj.UserMetadata["X-Amz-Meta-Compressed"]
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
	w.Header().Set("Content-Type", "application/json")

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
