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
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// DefaultTransport - this default transport is similar to
// http.DefaultTransport but with additional param  DisableCompression
// is set to true to avoid decompressing content with 'gzip' encoding.
var DefaultTransport http.RoundTripper = &http.Transport{
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
		RootCAs:    globalRootCAs,
	},
}
