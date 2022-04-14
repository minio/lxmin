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
	"net/http"
	"path"
	"time"

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
	Clnt        *minio.Client
	Bucket      string
	StagingRoot string
	TLSCerts    *certs.Manager
	RootCAs     *x509.CertPool
	NotifyClnt  *http.Client
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
	for obj := range l.Clnt.ListObjects(context.Background(), l.Bucket, minio.ListObjectsOptions{
		Prefix:       prefix,
		WithVersions: true,
	}) {
		if obj.Err != nil {
			return obj.Err
		}
		opts.VersionID = obj.VersionID
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
