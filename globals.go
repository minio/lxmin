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
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/pkg/certs"
)

var globalContext *lxminContext

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

	globalContext = &lxminContext{
		Clnt:        s3Client,
		Bucket:      c.String("bucket"),
		StagingRoot: c.String("staging"),
	}

	if c.String("cert") != "" || c.String("key") != "" {
		tlsCerts, err := certs.NewManager(context.Background(), c.String("cert"), c.String("key"), loadX509KeyPair)
		if err != nil {
			return err
		}

		publicCerts, err := parsePublicCertFile(c.String("cert"))
		if err != nil {
			return err
		}

		rootCAs, err := certs.GetRootCAs(c.String("capath"))
		if err != nil {
			return err
		}

		for _, cert := range publicCerts {
			rootCAs.AddCert(cert)
		}

		globalContext.TLSCerts = tlsCerts
		globalContext.RootCAs = rootCAs
	}

	globalContext.NotifyEndpoint = c.String("notify-endpoint")
	globalContext.NotifyClnt = &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          256,
			MaxIdleConnsPerHost:   16,
			ResponseHeaderTimeout: time.Minute,
			IdleConnTimeout:       time.Minute,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 10 * time.Second,
			// Set this value so that the underlying transport round-tripper
			// doesn't try to auto decode the body of objects with
			// content-encoding set to `gzip`.
			//
			// Refer:
			//    https://golang.org/src/net/http/transport.go?h=roundTrip#L1843
			DisableCompression: true,
			TLSClientConfig: &tls.Config{
				// Can't use SSLv3 because of POODLE and BEAST
				// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
				// Can't use TLSv1.1 because of RC4 cipher usage
				MinVersion: tls.VersionTLS12,
				RootCAs:    globalContext.RootCAs,
			},
		},
	}

	return nil
}
