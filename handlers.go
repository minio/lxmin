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
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
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

var errNotifyEpRequired = errors.New("a notification endpoint is required")

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
	Instance   string            `json:"instance,omitempty"`
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

func performBackup(instance, backupName string, tagsMap map[string]string, partSize int64, startedAt time.Time, notifyEndpoint string, r *http.Request) error {
	notifyEvent(eventInfo{
		OpType:    Backup,
		State:     Started,
		Name:      backupName,
		Instance:  instance,
		StartedAt: &startedAt,
		RawURL:    r.URL.String(),
	}, notifyEndpoint)

	bkReader := &backupReader{Started: true}
	globalBackupState.Store(backupName, bkReader)
	defer globalBackupState.Pop(backupName)

	// Export profiles to files.

	profiles, err := listProfiles(instance)
	if err != nil {
		return err
	}

	if len(profiles) > 1000 {
		return fmt.Errorf("More than a 1000 profiles per instance not supported.")
	}

	prInfo := make(map[string]profileInfo, len(profiles))
	for pno, profile := range profiles {
		// Profiles are numbered because their order matters - settings
		// in the later profiles override those from earlier profiles.
		profileFile := fmt.Sprintf("%s_profile_%03d_%s.yaml", backupName, pno, profile)
		profilePath := path.Join(globalContext.StagingRoot, profileFile)

		prSize, err := exportProfile(profile, profilePath)
		if err != nil {
			return err
		}

		prInfo[profile] = profileInfo{
			FileName: profileFile,
			Size:     prSize,
		}
	}

	// Export instance to tarball

	instanceBkpFilename := backupName + "_instance.tar.gz"
	localPath := path.Join(globalContext.StagingRoot, instanceBkpFilename)
	optimized := r.Form.Get("optimize") == "true"
	instanceSize, err := exportInstance(instance, localPath, optimized)
	if err != nil {
		return err
	}

	// Upload instance tarball to MinIO.

	bkReader.Size = instanceSize
	globalBackupState.Store(backupName, bkReader)

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

	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(localPath)

	bkp := backup{instance: instance, backupName: backupName}
	_, err = globalContext.Clnt.PutObject(context.Background(), globalContext.Bucket, bkp.key(), f, instanceSize, opts)
	if err != nil {
		return err
	}

	// Upload profiles to MinIO.
	for _, profile := range profiles {
		err := func() error {
			profileFile := prInfo[profile].FileName
			size := prInfo[profile].Size
			fpath := path.Join(globalContext.StagingRoot, profileFile)
			f, err := os.Open(fpath)
			if err != nil {
				return err
			}
			defer f.Close()
			defer os.Remove(fpath)

			opts := minio.PutObjectOptions{
				UserTags:    tagsMap,
				PartSize:    uint64(partSize),
				ContentType: mime.TypeByExtension(".yaml"),
			}
			_, err = globalContext.Clnt.PutObject(context.Background(), globalContext.Bucket, path.Join(instance, profileFile), f, size, opts)
			if err != nil {
				return fmt.Errorf("Error uploading file %s: %v", fpath, err)
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}

	completedAt := time.Now()
	notifyEvent(eventInfo{
		OpType:      Backup,
		State:       Success,
		Name:        backupName,
		Instance:    instance,
		StartedAt:   &startedAt,
		CompletedAt: &completedAt,
		RawURL:      r.URL.String(),
	}, notifyEndpoint)
	return err
}

func performRestore(instance, backupName string, startedAt time.Time, notifyEndpoint string, r *http.Request) error {
	notifyEvent(eventInfo{
		OpType:    Restore,
		State:     Started,
		Name:      backupName,
		Instance:  instance,
		StartedAt: &startedAt,
		RawURL:    r.URL.String(),
	}, notifyEndpoint)

	bkp := backup{instance: instance, backupName: backupName}

	// Fetch restore info
	resInfo, err := globalContext.fetchRestoreInfo(bkp)
	if err != nil {
		return err
	}

	// Download profiles
	for _, pkey := range resInfo.profileKeys {
		err := globalContext.downloadItem(pkey, nil)
		if err != nil {
			return fmt.Errorf("Error downloading profile file %s: %v", pkey, err)
		}
	}

	// Download instance backup
	if err := globalContext.downloadItem(bkp.key(), nil); err != nil {
		return fmt.Errorf("Error downloading instance backup %s: %v", bkp.key(), err)
	}

	// Fetch existing profiles on the system
	existingProfiles, err := fetchExistingProfiles()
	if err != nil {
		return err
	}

	// Restore profiles - skip those that already exist.
	for i, pf := range resInfo.profiles {
		err := restoreProfile(globalContext, pf, resInfo.profileKeys[i], existingProfiles)
		if _, ok := err.(warnMsgErr); ok {
			// Skip warning that profile was not replaced for now.
			continue
		} else if err != nil {
			return err
		}
	}

	// Restore instance
	_, err = restoreInstance(globalContext, bkp)
	if err != nil {
		return err
	}

	completedAt := time.Now()

	notifyEvent(eventInfo{
		OpType:      Restore,
		State:       Success,
		Name:        backupName,
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

	if notifyEndpoint == "" {
		notifyEndpoint = globalContext.NotifyEndpoint
	}

	if notifyEndpoint == "" {
		writeErrorResponse(w, errNotifyEpRequired)
		return
	}

	go func() {
		startedAt := time.Now()
		if err := performRestore(instance, backup, startedAt, notifyEndpoint, r); err != nil {
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

	if notifyEndpoint == "" {
		notifyEndpoint = globalContext.NotifyEndpoint
	}

	if notifyEndpoint == "" {
		writeErrorResponse(w, errNotifyEpRequired)
		return
	}

	backup := "backup_" + time.Now().Format("2006-01-02-15-0405")
	go func() {
		startedAt := time.Now()
		if err := performBackup(instance, backup, tagsSet.ToMap(), partSize, startedAt, notifyEndpoint, r); err != nil {
			failedAt := time.Now()
			notifyEvent(eventInfo{
				OpType:    Backup,
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
	backupName := vars["backup"]

	if instance == "" {
		writeErrorResponse(w, errors.New("instance name cannot be empty"))
		return
	}

	if backupName == "" {
		writeErrorResponse(w, errors.New("backup name cannot be empty"))
		return
	}

	bkp := backup{
		instance:   instance,
		backupName: backupName,
	}

	err := globalContext.DeleteBackup(bkp)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	writeSuccessResponse(w, nil, true)
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	instance := vars["name"]
	backupName := vars["backup"]

	if instance == "" {
		writeErrorResponse(w, errors.New("instance name cannot be empty"))
		return
	}

	if backupName == "" {
		writeErrorResponse(w, errors.New("backup name cannot be empty"))
		return
	}

	if reader := globalBackupState.Get(backupName); reader != nil {
		state := "generating"
		progress := atomic.LoadInt64(&reader.Progress)
		if reader.Started && progress > 0 {
			state = "uploading"
		}
		writeSuccessResponse(w, backupInfo{
			Name:     backupName,
			Size:     reader.Size,
			State:    state,
			Progress: &progress,
		}, true)
		return
	}

	bkp := backup{
		instance:   instance,
		backupName: backupName,
	}

	tags, err := globalContext.GetTags(bkp)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	meta, err := globalContext.GetMetadata(bkp)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	optimized := meta.UserMetadata["Optimized"] == "true"
	compressed := meta.UserMetadata["Compressed"] == "true"

	info := backupInfo{
		Name:       backupName,
		Created:    &meta.LastModified,
		Size:       meta.Size,
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

	// Allow obtaining all backups
	if instance == "*" {
		instance = ""
	}

	backups, err := globalContext.ListBackups(instance)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	writeSuccessResponse(w, backups, true)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	// A very simple health check.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(http.StatusOK)
}
