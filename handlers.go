// Copyright (c) 2015-2022 MinIO, Inc.
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
	"net/url"
	"os"
	"os/exec"
	"path"
	"strconv"
	"sync"
	"sync/atomic"
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

type backupInfo struct {
	Name       string            `json:"name"`
	Created    *time.Time        `json:"created,omitempty"`
	Size       int64             `json:"size,omitempty"`
	Optimized  *bool             `json:"optimized,omitempty"`
	Compressed *bool             `json:"compressed,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	State      string            `json:"state,omitempty"`
	Progress   *int64            `json:"progress,omitempty"`
}

type backupReader struct {
	Started  bool
	Size     int64
	Progress int64
}

func (bk *backupReader) Read(b []byte) (int, error) {
	atomic.AddInt64(&bk.Progress, int64(len(b)))
	return len(b), nil
}

type backupState struct {
	sync.RWMutex
	backups map[string]*backupReader
}

func (s *backupState) Store(bname string, rk *backupReader) {
	s.Lock()
	defer s.Unlock()

	s.backups[bname] = rk
}

func (s *backupState) Pop(bname string) {
	s.Lock()
	defer s.Unlock()

	delete(s.backups, bname)
}

func (s *backupState) Get(bname string) *backupReader {
	s.RLock()
	defer s.RUnlock()

	return s.backups[bname]
}

var globalBackupState = &backupState{
	backups: map[string]*backupReader{},
}

func performBackup(instance, backup string, tagsMap map[string]string, partSize int64, startedAt time.Time, r *http.Request) error {
	notifyEndpoint, err := url.QueryUnescape(r.Form.Get("notifyEndpoint"))
	if err != nil {
		return err
	}

	localPath := path.Join(globalContext.StagingRoot, backup)
	cmd := exec.Command("lxc", "export", instance, localPath)
	optimized := r.Form.Get("optimize") == "true"
	if optimized {
		cmd = exec.Command("lxc", "export", "--optimized-storage", instance, localPath)
	}
	cmd.Stdout = ioutil.Discard

	bkReader := &backupReader{Started: true}
	globalBackupState.Store(backup, bkReader)
	defer globalBackupState.Pop(backup)

	if err := cmd.Run(); err != nil {
		return err
	}

	f, err := os.Open(localPath)
	if err != nil {
		return err
	}

	defer os.Remove(localPath)

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	bkReader.Size = fi.Size()
	globalBackupState.Store(backup, bkReader)

	usermetadata := map[string]string{}
	// Save additional information if the backup is optimized or not.
	usermetadata["optimized"] = strconv.FormatBool(optimized)
	usermetadata["compressed"] = "true" // This is always true.

	opts := minio.PutObjectOptions{
		UserTags:     tagsMap,
		PartSize:     uint64(partSize),
		UserMetadata: usermetadata,
		ContentType:  mime.TypeByExtension(".tar.gz"),
		Progress:     bkReader,
	}
	_, err = globalContext.Clnt.PutObject(context.Background(), globalContext.Bucket, path.Join(instance, backup), f, fi.Size(), opts)
	f.Close()
	if err == nil {
		completedAt := time.Now()
		notifyEvent(eventInfo{
			OpType:      Backup,
			State:       Success,
			Name:        backup,
			Instance:    instance,
			StartedAt:   &startedAt,
			CompletedAt: &completedAt,
			RawURL:      r.URL.String(),
		}, notifyEndpoint)
	}
	return err
}

func performRestore(instance, backup string, startedAt time.Time, r *http.Request) error {
	notifyEndpoint, err := url.QueryUnescape(r.Form.Get("notifyEndpoint"))
	if err != nil {
		return err
	}

	opts := minio.GetObjectOptions{}
	obj, err := globalContext.Clnt.GetObject(context.Background(), globalContext.Bucket, path.Join(instance, backup), opts)
	if err != nil {
		return err
	}

	localPath := path.Join(globalContext.StagingRoot, backup)

	os.Remove(localPath) // remove any existing file.

	w, err := os.Create(localPath)
	if err != nil {
		obj.Close()
		return err
	}
	io.Copy(w, obj)
	obj.Close()

	cmd := exec.Command("lxc", "import", localPath)
	cmd.Stdout = ioutil.Discard

	if err := cmd.Run(); err != nil {
		os.Remove(localPath)
		return err
	}

	cmd = exec.Command("lxc", "start", instance)
	cmd.Stdout = ioutil.Discard
	if err := cmd.Run(); err != nil {
		os.Remove(backup)
		return err
	}

	if err := os.Remove(backup); err != nil {
		return err
	}

	completedAt := time.Now()

	notifyEvent(eventInfo{
		OpType:      Restore,
		State:       Success,
		Name:        backup,
		Instance:    instance,
		StartedAt:   &startedAt,
		CompletedAt: &completedAt,
		RawURL:      r.URL.String(),
	}, notifyEndpoint)

	return nil
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

	notifyEndpoint, err := url.QueryUnescape(r.Form.Get("notifyEndpoint"))
	if err != nil {
		writeErrorResponse(w, errors.New("invalid notifyEndpoint"))
		return
	}

	go func() {
		startedAt := time.Now()
		if err := performRestore(instance, backup, startedAt, r); err != nil {
			failedAt := time.Now()
			notifyEvent(eventInfo{
				OpType:    Restore,
				State:     Failed,
				Name:      backup,
				Instance:  instance,
				StartedAt: &startedAt,
				FailedAt:  &failedAt,
				Error:     err,
				RawURL:    r.URL.String(),
			}, notifyEndpoint)
			log.Println(err)
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

	tagsSet, err := tags.Parse(r.Form.Get("tags"), true)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	notifyEndpoint, err := url.QueryUnescape(r.Form.Get("notifyEndpoint"))
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	backup := "backup_" + time.Now().Format("2006-01-02-15-0405") + ".tar.gz"
	go func() {
		startedAt := time.Now()
		if err := performBackup(instance, backup, tagsSet.ToMap(), partSize, startedAt, r); err != nil {
			failedAt := time.Now()
			notifyEvent(eventInfo{
				OpType:    Restore,
				State:     Failed,
				Name:      backup,
				Instance:  instance,
				StartedAt: &startedAt,
				FailedAt:  &failedAt,
				Error:     err,
				RawURL:    r.URL.String(),
			}, notifyEndpoint)
			log.Println(err)
		}
	}()

	optimized := r.Form.Get("optimize") == "true"
	compressed := true

	sresp := &successResponse{
		Metadata: backupInfo{
			Name:       backup,
			Optimized:  &optimized,
			Compressed: &compressed,
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
	for obj := range globalContext.Clnt.ListObjects(context.Background(), globalContext.Bucket, minio.ListObjectsOptions{
		Prefix:       prefix,
		WithVersions: true,
	}) {
		if obj.Err != nil {
			switch minio.ToErrorResponse(obj.Err).Code {
			case "NotImplemented":
				// fallback for ListObjectVersions not implemented.
				if err := globalContext.Clnt.RemoveObject(context.Background(), globalContext.Bucket, prefix, opts); err != nil {
					writeErrorResponse(w, err)
					return
				}
			default:
				writeErrorResponse(w, obj.Err)
				return
			}
		} else {
			opts.VersionID = obj.VersionID
			if err := globalContext.Clnt.RemoveObject(context.Background(), globalContext.Bucket, obj.Key, opts); err != nil {
				writeErrorResponse(w, err)
				return
			}
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

	if reader := globalBackupState.Get(backup); reader != nil {
		state := "generating"
		progress := atomic.LoadInt64(&reader.Progress)
		if reader.Started && progress > 0 {
			state = "uploading"
		}
		writeSuccessResponse(w, backupInfo{
			Name:     backup,
			Size:     reader.Size,
			State:    state,
			Progress: &progress,
		}, true)
		return
	}

	opts := minio.GetObjectTaggingOptions{}
	tags, err := globalContext.Clnt.GetObjectTagging(context.Background(), globalContext.Bucket, path.Join(instance, backup), opts)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	sopts := minio.StatObjectOptions{}
	obj, err := globalContext.Clnt.StatObject(context.Background(), globalContext.Bucket, path.Join(instance, backup), sopts)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	optimized := obj.UserMetadata["Optimized"] == "true"
	compressed := obj.UserMetadata["Compressed"] == "true"

	info := backupInfo{
		Name:       backup,
		Created:    &obj.LastModified,
		Size:       obj.Size,
		Optimized:  &optimized,
		Compressed: &compressed,
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
	for obj := range globalContext.Clnt.ListObjects(context.Background(), globalContext.Bucket, minio.ListObjectsOptions{
		Prefix:       instance,
		Recursive:    true,
		WithMetadata: true,
	}) {
		if obj.Err != nil {
			writeErrorResponse(w, obj.Err)
			return
		}
		optimized := obj.UserMetadata["X-Amz-Meta-Optimized"] == "true"
		compressed := obj.UserMetadata["X-Amz-Meta-Compressed"] == "true"
		backups = append(backups, backupInfo{
			Name:       path.Base(obj.Key),
			Created:    &obj.LastModified,
			Size:       obj.Size,
			Optimized:  &optimized,
			Compressed: &compressed,
		})
	}

	writeSuccessResponse(w, backups, true)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	// A very simple health check.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(http.StatusOK)
}
