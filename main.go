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
	"log"
	"os"
	"os/exec"

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
`

var appCmds = []cli.Command{
	backupCmd,
	restoreCmd,
	infoCmd,
	listCmd,
	deleteCmd,
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
		log.Fatal(err)
	}
}
