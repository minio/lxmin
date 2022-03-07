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
	"net/url"

	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/pkg/certs"
)

var (
	globalS3Clnt     *minio.Client
	globalBucket     string
	globalTLSCerts   *certs.Manager
	globalRootCAs    *x509.CertPool
	globalNotifyClnt *http.Client
)

// Set global states. NOTE: It is deliberately kept monolithic to ensure we dont miss out any flags.
func setGlobalsFromContext(c *cli.Context) error {
	u, err := url.Parse(c.String("endpoint"))
	if err != nil {
		return err
	}

	s3Client, err := minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(c.String("access-key"), c.String("secret-key"), ""),
		Secure: u.Scheme == "https",
	})
	if err != nil {
		return err
	}

	globalS3Clnt = s3Client
	globalBucket = c.String("bucket")
	globalTLSCerts, err = certs.NewManager(context.Background(), c.String("cert"), c.String("key"), loadX509KeyPair)
	if err != nil {
		return err
	}

	publicCerts, err := parsePublicCertFile(c.String("cert"))
	if err != nil {
		return err
	}

	globalRootCAs, err = certs.GetRootCAs(c.String("capath"))
	if err != nil {
		return err
	}

	for _, cert := range publicCerts {
		globalRootCAs.AddCert(cert)
	}

	globalNotifyClnt = &http.Client{
		Transport: DefaultTransport,
	}

	return nil
}
