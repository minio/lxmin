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
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"time"

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
		Usage:  "run as HTTPs REST API service",
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
  LXMIN_ENDPOINT      endpoint for MinIO server
  LXMIN_BUCKET        bucket to save/restore backup(s)
  LXMIN_ACCESS_KEY    access key credential
  LXMIN_SECRET_KEY    secret key credential
  LXMIN_ADDRESS       run as HTTPs REST API service
`

var appCmds = []cli.Command{
	backupCmd,
	restoreCmd,
	infoCmd,
	listCmd,
	deleteCmd,
}

func mainHTTP(c *cli.Context) error {
	if !c.IsSet("address") {
		return nil
	}

	if err := setGlobalsFromContext(c); err != nil {
		return err
	}

	r := mux.NewRouter()
	r.HandleFunc("/1.0/instances/{name}/backups", listHandler).
		Methods(http.MethodGet)
	r.HandleFunc("/1.0/instances/{name}/backups/{backup}", infoHandler).
		Methods(http.MethodHead)
	// r.HandleFunc("/1.0/instances/{name}/backups", backupHandler).
	// 	Methods(http.MethodPost)
	// r.HandleFunc("/1.0/instances/{name}/backups/{backup}", deleteHandler).
	// 	Methods(http.MethodDelete)
	// r.HandleFunc("/1.0/instances/{name}/backups/{backup}", restoreHandler).
	// 	Methods(http.MethodPost)
	r.HandleFunc("/1.0/health", healthHandler)

	srv := &http.Server{
		Handler:     r,
		Addr:        c.String("address"),
		IdleTimeout: time.Second * 60,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
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
		if !c.Args().Present() {
			cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
		}
		if c.Bool("help") {
			cli.ShowAppHelpAndExit(c, 0) // last argument is exit code
		}
		_, err := exec.LookPath("lxc")
		return err
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalln(err)
	}
}
