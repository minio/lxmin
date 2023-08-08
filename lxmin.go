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
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cheggaaa/pb/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/tags"
	"github.com/minio/pkg/certs"
)

type backupMeta struct {
	Size         int64
	LastModified time.Time
	UserMetadata minio.StringMap
}

type lxminContext struct {
	Clnt           *minio.Client
	Bucket         string
	StagingRoot    string
	TLSCerts       *certs.Manager
	RootCAs        *x509.CertPool
	NotifyClnt     *http.Client
	NotifyEndpoint string
}

// GetTags - fetch tags on the backup.
func (l *lxminContext) GetTags(bkp backup) (*tags.Tags, error) {
	opts := minio.GetObjectTaggingOptions{}
	return l.Clnt.GetObjectTagging(context.Background(), l.Bucket, bkp.key(), opts)
}

// GetMetadata - get backup metadata.
func (l *lxminContext) GetMetadata(bkp backup) (backupMeta, error) {
	sopts := minio.StatObjectOptions{}
	obj, err := l.Clnt.StatObject(context.Background(), l.Bucket, bkp.key(), sopts)
	if err != nil {
		return backupMeta{}, err
	}

	return backupMeta{
		Size:         obj.Size,
		LastModified: obj.LastModified,
		UserMetadata: obj.UserMetadata,
	}, nil
}

// listAndDelete - CAUTION: deletes everything at the prefix.
func (l *lxminContext) listAndDelete(prefix string) error {
	opts := minio.RemoveObjectOptions{}

	resCh := l.Clnt.ListObjects(context.Background(), l.Bucket, minio.ListObjectsOptions{
		Prefix:       prefix,
		WithVersions: true,
	})

	isVersioned := true
	for obj := range resCh {
		if obj.Err != nil {
			switch minio.ToErrorResponse(obj.Err).Code {
			case "NotImplemented":
				// fallback for ListObjectVersions not implemented.
				resCh = l.Clnt.ListObjects(context.Background(), l.Bucket, minio.ListObjectsOptions{
					Prefix: prefix,
				})
				isVersioned = false
				continue
			default:
				return obj.Err
			}
		}

		if isVersioned {
			// When listing is versioned, set the version ID for
			// delete.
			opts.VersionID = obj.VersionID
		}
		if err := l.Clnt.RemoveObject(context.Background(), l.Bucket, obj.Key, opts); err != nil {
			return err
		}
	}

	return nil
}

// DeleteBackup - deletes a particular backup of an instance in MinIO.
func (l *lxminContext) DeleteBackup(bkp backup) error {
	prefix := bkp.prefix()
	return l.listAndDelete(prefix)
}

// DeleteAllBackups - deletes all backups for the given instance.
func (l *lxminContext) DeleteAllBackups(instance string) error {
	prefix := path.Clean(instance) + "/"
	return l.listAndDelete(prefix)
}

// ListItems - lists all items at the given prefix.
func (l *lxminContext) ListItems(prefix string) ([]minio.ObjectInfo, error) {
	var oi []minio.ObjectInfo
	for obj := range l.Clnt.ListObjects(context.Background(), l.Bucket, minio.ListObjectsOptions{
		Prefix:       prefix,
		Recursive:    true,
		WithMetadata: true,
	}) {
		if obj.Err != nil {
			return nil, obj.Err
		}

		oi = append(oi, obj)
	}
	return oi, nil
}

func objToBackupInfo(obj minio.ObjectInfo, instance string) backupInfo {
	backupName := strings.TrimSuffix(path.Base(obj.Key), "_instance.tar.gz")

	optimized := obj.UserMetadata["X-Amz-Meta-Optimized"] == "true"
	compressed := obj.UserMetadata["X-Amz-Meta-Compressed"] == "true"
	return backupInfo{
		Instance:   instance,
		Name:       backupName,
		Created:    &obj.LastModified,
		Size:       obj.Size,
		Optimized:  &optimized,
		Compressed: &compressed,
	}
}

// ListBackups - lists available backups in MinIO. If `instance` is empty lists
// backups for all instances.
func (l *lxminContext) ListBackups(instance string) ([]backupInfo, error) {
	var backups []backupInfo
	prefix := ""
	if instance != "" {
		prefix = path.Clean(instance) + "/"
	}
	backupItems, err := l.ListItems(prefix)
	if err != nil {
		return nil, err
	}

	for _, obj := range backupItems {
		// Do not consider the profiles in the listing.
		if !strings.HasSuffix(obj.Key, "_instance.tar.gz") {
			continue
		}

		inst := instance
		if instance == "" {
			inst = path.Dir(obj.Key)
		}

		backups = append(backups, objToBackupInfo(obj, inst))
	}
	return backups, nil
}

type restoreInfo struct {
	profiles    []string
	profileKeys []string
	totalSize   int64
}

func (l *lxminContext) fetchRestoreInfo(bkp backup) (ri restoreInfo, err error) {
	items, err := l.ListItems(path.Join(bkp.instance, bkp.backupName+"_profile_"))
	if err != nil {
		return ri, fmt.Errorf("Error listing profiles for backup %s (instance: %s): %v", bkp.backupName, bkp.instance, err)
	}

	for pno, obj := range items {
		expectedProfilePrefix := fmt.Sprintf("%s_profile_%03d_", bkp.backupName, pno)
		profileName := strings.TrimPrefix(
			strings.TrimSuffix(path.Base(obj.Key), ".yaml"),
			expectedProfilePrefix,
		)

		// Validate the profile object name.
		if !strings.HasPrefix(path.Base(obj.Key), expectedProfilePrefix) || !strings.HasSuffix(obj.Key, ".yaml") || profileName == "" {
			return ri, fmt.Errorf("Unexpected profile file found: %s", obj.Key)
		}

		ri.totalSize += obj.Size
		ri.profiles = append(ri.profiles, profileName)
		ri.profileKeys = append(ri.profileKeys, obj.Key)
	}

	oi, err := l.Clnt.StatObject(context.Background(), l.Bucket, bkp.key(), minio.StatObjectOptions{})
	if err != nil {
		return ri, fmt.Errorf("Error getting instance backup file info: %v", err)
	}

	ri.totalSize += oi.Size
	return ri, nil
}

func (l *lxminContext) downloadItem(objPath string, bar *pb.ProgressBar) error {
	fpath := path.Join(l.StagingRoot, path.Base(objPath))
	var w io.Writer
	if bar != nil {
		barWriter, err := newBarUpdateWriter(fpath, bar, tmplDl)
		if err != nil {
			return err
		}
		defer barWriter.Close()
		w = barWriter
	} else {
		f, err := os.Create(fpath)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}

	obj, err := l.Clnt.GetObject(context.Background(), l.Bucket, objPath, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer obj.Close()

	_, err = io.Copy(w, obj)
	return err
}

type backup struct {
	instance, backupName string
}

func (b *backup) key() string {
	return path.Join(b.instance, b.backupName+"_instance.tar.gz")
}

// prefix - returns the prefix at which all backup files are present.
func (b *backup) prefix() string {
	return path.Join(b.instance, b.backupName)
}

type backupOpts struct {
	TagsSet   *tags.Tags
	PartSize  int64
	Optimized bool
}
