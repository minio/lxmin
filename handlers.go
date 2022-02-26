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
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gorilla/mux"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/tags"
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
	Metadata interface{}  `json:"metadata,omitempty"`
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

type bkTime struct {
	time.Time
}

func (t bkTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return nil, nil
	}
	return json.Marshal(time.Time(t.Time))
}

type backupInfo struct {
	Name       string            `json:"name"`
	Created    bkTime            `json:"created,omitempty"`
	Size       int64             `json:"size,omitempty"`
	Optimized  bool              `json:"optimized"`
	Compressed bool              `json:"compressed"`
	Tags       map[string]string `json:"tags,omitempty"`
}

func performBackup(instance, backup string, tagsMap map[string]string, partSize int64, r *http.Request) error {
	cmd := exec.Command("lxc", "export", instance, backup)
	optimized := r.Form.Get("optimize") == "true"
	if optimized {
		cmd = exec.Command("lxc", "export", "--optimized-storage", instance, backup)
	}
	cmd.Stdout = ioutil.Discard

	if err := cmd.Run(); err != nil {
		return err
	}

	f, err := os.Open(backup)
	if err != nil {
		return err
	}

	defer os.Remove(backup)

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	usermetadata := map[string]string{}
	// Save additional information if the backup is optimized or not.
	usermetadata["optimized"] = strconv.FormatBool(optimized)
	usermetadata["compressed"] = "true" // This is always true.

	opts := minio.PutObjectOptions{
		UserTags:     tagsMap,
		PartSize:     uint64(partSize),
		UserMetadata: usermetadata,
		ContentType:  mime.TypeByExtension(".tar.gz"),
	}
	_, err = globalS3Clnt.PutObject(context.Background(), globalBucket, path.Join(instance, backup), f, fi.Size(), opts)
	f.Close()
	return err
}

func performRestore(instance, backup string, r *http.Request) error {
	opts := minio.GetObjectOptions{}
	obj, err := globalS3Clnt.GetObject(context.Background(), globalBucket, path.Join(instance, backup), opts)
	if err != nil {
		return err
	}

	os.Remove(backup) // remove any existing file.

	w, err := os.Create(backup)
	if err != nil {
		obj.Close()
		return err
	}
	io.Copy(w, obj)
	obj.Close()

	cmd := exec.Command("lxc", "import", backup)
	cmd.Stdout = ioutil.Discard

	if err := cmd.Run(); err != nil {
		os.Remove(backup)
		return err
	}

	cmd = exec.Command("lxc", "start", instance)
	cmd.Stdout = ioutil.Discard
	if err := cmd.Run(); err != nil {
		os.Remove(backup)
		return err
	}

	return os.Remove(backup)
}

func restoreHandler(w http.ResponseWriter, r *http.Request) {
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

	if err := checkInstance(instance); err != nil {
		writeErrorResponse(w, err)
		return
	}

	go func() {
		rerr := performRestore(instance, backup, r)
		if rerr != nil {
			log.Println(rerr)
		}
	}()

	sresp := &successResponse{
		Status: "Operation created",
		Code:   100,
		Type:   AsyncResponse,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	json.NewEncoder(w).Encode(sresp)

}

func backupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instance := vars["name"]

	if instance == "" {
		writeErrorResponse(w, errors.New("instance name cannot be empty"))
		return
	}

	partSize, err := strconv.ParseInt(r.Form.Get("partSize"), 10, 64)
	if err != nil && r.Form.Get("partSize") != "" {
		writeErrorResponse(w, err)
		return
	}
	if partSize == 0 {
		partSize = 64 * humanize.MiByte
	}

	tagsHdr := r.Header.Get("x-amz-tagging")
	tagsSet, err := tags.Parse(tagsHdr, true)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	backup := "backup_" + time.Now().Format("2006-01-02-15-0405") + ".tar.gz"
	go func() {
		berr := performBackup(instance, backup, tagsSet.ToMap(), partSize, r)
		if berr != nil {
			log.Println(berr)
		}
	}()

	optimized := r.Form.Get("optimize") == "true"

	sresp := &successResponse{
		Metadata: backupInfo{
			Name:       backup,
			Optimized:  optimized,
			Compressed: true,
		},
		Status: "Operation created",
		Code:   100,
		Type:   AsyncResponse,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	json.NewEncoder(w).Encode(sresp)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
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

	prefix := path.Join(path.Clean(instance), backup)

	opts := minio.RemoveObjectOptions{}
	for obj := range globalS3Clnt.ListObjects(context.Background(), globalBucket, minio.ListObjectsOptions{
		Prefix:       prefix,
		WithVersions: true,
	}) {
		if obj.Err != nil {
			writeErrorResponse(w, obj.Err)
			return
		}
		opts.VersionID = obj.VersionID
		if err := globalS3Clnt.RemoveObject(context.Background(), globalBucket, obj.Key, opts); err != nil {
			writeErrorResponse(w, err)
			return
		}
	}

	writeSuccessResponse(w, nil, true)
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
		Created:    bkTime{obj.LastModified},
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
			Created:    bkTime{obj.LastModified},
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
