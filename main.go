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
	"crypto/x509"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/minio/cli"
)

const (
	tmplUp = `Uploading %s {{ bar . "┃" "▓" "▓" "░" "┃"}} {{speed . "%%s/s" "? MiB/s"}}`
	tmplDl = `Downloading %s {{ bar . "┃" "▓" "▓" "░" "┃"}} {{speed . "%%s/s" "? MiB/s"}}`
)

var globalFlags = []cli.Flag{
	cli.StringFlag{
		Name:   "endpoint",
		EnvVar: "LXMIN_ENDPOINT",
		Usage:  "endpoint for MinIO server",
	},
	cli.StringFlag{
		Name:   "bucket",
		EnvVar: "LXMIN_BUCKET",
		Usage:  "bucket to save/restore backup(s)",
	},
	cli.StringFlag{
		Name:   "access-key",
		EnvVar: "LXMIN_ACCESS_KEY",
		Usage:  "access key credential",
	},
	cli.StringFlag{
		Name:   "secret-key",
		EnvVar: "LXMIN_SECRET_KEY",
		Usage:  "secret key credential",
	},
	cli.StringFlag{
		Name:   "address",
		EnvVar: "LXMIN_ADDRESS",
		Usage:  "enable TLS REST API service",
	},
	cli.StringFlag{
		Name:   "cert",
		EnvVar: "LXMIN_TLS_CERT",
		Usage:  "TLS server certificate",
	},
	cli.StringFlag{
		Name:   "key",
		EnvVar: "LXMIN_TLS_KEY",
		Usage:  "TLS server private key",
	},
	cli.StringFlag{
		Name:   "capath",
		EnvVar: "LXMIN_TLS_CAPATH",
		Usage:  "TLS trust certs for incoming clients",
	},
}

var helpTemplate = `NAME:
  {{.Name}} - {{.Usage}}

USAGE:
  {{.Name}} {{if .VisibleFlags}}[FLAGS] {{end}}COMMAND{{if .VisibleFlags}} [COMMAND FLAGS | -h]{{end}} [ARGUMENTS...]

COMMANDS:
  {{range .VisibleCommands}}{{join .Names ", "}}{{ "\t" }}{{.Usage}}
  {{end}}{{if .VisibleFlags}}
GLOBAL FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}{{end}}
ENVIRONMENT VARIABLES:
  LXMIN_ENDPOINT        endpoint for MinIO server
  LXMIN_BUCKET          bucket to save/restore backup(s)
  LXMIN_ACCESS_KEY      access key credential
  LXMIN_SECRET_KEY      secret key credential
  LXMIN_ADDRESS         run as HTTPs REST API service
`

var appCmds = []cli.Command{
	backupCmd,
	restoreCmd,
	infoCmd,
	listCmd,
	deleteCmd,
}

func authenticateTLSClientHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			writeErrorResponse(w, errors.New("no tls connection"))
			return
		}

		// A client may send a certificate chain such that we end up
		// with multiple peer certificates. However, we can only accept
		// a single client certificate. Otherwise, the certificate to
		// policy mapping would be ambigious.
		// However, we can filter all CA certificates and only check
		// whether they client has sent exactly one (non-CA) leaf certificate.
		peerCertificates := make([]*x509.Certificate, 0, len(r.TLS.PeerCertificates))
		for _, cert := range r.TLS.PeerCertificates {
			if cert.IsCA {
				continue
			}
			peerCertificates = append(peerCertificates, cert)
		}
		r.TLS.PeerCertificates = peerCertificates

		// Now, we have to check that the client has provided exactly one leaf
		// certificate that we can map to a policy.
		if len(r.TLS.PeerCertificates) == 0 {
			writeErrorResponse(w, errors.New("no client certificate provided"))
			return
		}

		if len(r.TLS.PeerCertificates) > 1 {
			writeErrorResponse(w, errors.New("more than one client certificate provided"))
			return
		}

		certificate := r.TLS.PeerCertificates[0]
		if _, err := certificate.Verify(x509.VerifyOptions{
			KeyUsages: []x509.ExtKeyUsage{
				x509.ExtKeyUsageClientAuth,
			},
			Roots: globalRootCAs,
		}); err != nil {
			writeErrorResponse(w, err)
			return
		}

		if err := r.ParseForm(); err != nil {
			writeErrorResponse(w, err)
			return
		}

		h.ServeHTTP(w, r)
	})
}

func mainHTTP(c *cli.Context) error {
	if c.Args().Present() {
		// With args present no need to start lxmin service.
		return nil
	}
	if !c.IsSet("address") {
		return errors.New("address cannot be empty please use '--address=:8000' to start lxmin as service")
	}

	if err := setGlobalsFromContext(c); err != nil {
		return err
	}

	r := mux.NewRouter()
	r.StrictSlash(false)
	r.SkipClean(true)

	r.HandleFunc("/1.0/instances/{name}/backups", listHandler).Methods(http.MethodGet)
	r.HandleFunc("/1.0/instances/{name}/backups", backupHandler).Methods(http.MethodPost)
	r.HandleFunc("/1.0/instances/{name}/backups/{backup}", infoHandler).Methods(http.MethodGet)
	r.HandleFunc("/1.0/instances/{name}/backups/{backup}", deleteHandler).Methods(http.MethodDelete)
	r.HandleFunc("/1.0/instances/{name}/backups/{backup}", restoreHandler).Methods(http.MethodPost)
	r.HandleFunc("/1.0/health", healthHandler).Methods(http.MethodGet, http.MethodHead)
	r.Use(authenticateTLSClientHandler)

	tlsConfig := &tls.Config{
		PreferServerCipherSuites: true,
		MinVersion:               tls.VersionTLS12,
		NextProtos:               []string{"http/1.1", "h2"},
		GetCertificate:           globalTLSCerts.GetCertificate,
		ClientAuth:               tls.RequestClientCert,
	}

	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		NotFound(nil).Render(w)
	})

	srv := &http.Server{
		Handler:     handlers.CompressHandler(handlers.LoggingHandler(os.Stdout, handlers.ProxyHeaders(r))),
		Addr:        c.String("address"),
		TLSConfig:   tlsConfig,
		IdleTimeout: time.Second * 60,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		log.Println("Server listening on", srv.Addr)

		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			log.Fatalln(err)
		}

		defer ln.Close()

		if err := srv.Serve(tls.NewListener(ln, tlsConfig)); err != nil && err != http.ErrServerClosed {
			log.Fatalln(err)
		}
	}()

	ch := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(ch, os.Interrupt)

	// Block until we receive our signal.
	<-ch

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)

	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	return nil
}

func main() {
	app := cli.NewApp()
	app.Copyright = "MinIO, Inc."
	app.Usage = "backup and restore LXC instances with MinIO"
	app.CustomAppHelpTemplate = helpTemplate
	app.HideHelpCommand = true
	app.HideVersion = true
	app.Flags = globalFlags
	app.Commands = appCmds
	app.Before = func(c *cli.Context) error {
		if c.Bool("help") {
			cli.ShowAppHelpAndExit(c, 0) // last argument is exit code
		}
		_, err := exec.LookPath("lxc")
		return err
	}

	// Start http service if configured.
	app.Action = mainHTTP

	if err := app.Run(os.Args); err != nil {
		log.Fatalln(err)
	}
}
