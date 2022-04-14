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
	"errors"
	"fmt"
	"strings"

	"github.com/minio/cli"
)

var deleteFlags = []cli.Flag{
	cli.BoolFlag{
		Name:  "all",
		Usage: "delete all backups for an instance, only valid for 'delete' command",
	},
	cli.BoolFlag{
		Name:  "force",
		Usage: "allow all backups to be deleted, only valid when '--all' is specified",
	},
}

var deleteCmd = cli.Command{
	Name:    "delete",
	Aliases: []string{"rm"},
	Usage:   "deletes a specific backup by 'name' for an instance from MinIO",
	Action:  deleteMain,
	Before:  setGlobalsFromContext,
	Flags:   append(deleteFlags, globalFlags...),
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [FLAGS] INSTANCENAME BACKUPNAME

TIP:
   --all --force flags can be provided without 'BACKUPNAME' to delete all backups.

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
EXAMPLES:
  1. Delete a backup 'backup_2022-02-16-04-1040' for instance 'u2':
     {{.Prompt}} {{.HelpName}} u2 backup_2022-02-16-04-1040
`,
}

func deleteMain(c *cli.Context) error {
	if len(c.Args()) > 2 {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	instance := strings.TrimSpace(c.Args().Get(0))
	if instance == "" {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	backupName := strings.TrimSpace(c.Args().Get(1))
	deleteAll := c.Bool("all") && c.Bool("force")
	if backupName == "" && !deleteAll {
		return errors.New("backup name is not optional without --all")
	}

	var err error
	if backupName != "" {
		bkp := backup{instance: instance, backupName: backupName}
		err = globalContext.DeleteBackup(bkp)
	} else {
		err = globalContext.DeleteAllBackups(instance)
	}
	if err != nil {
		return err
	}

	if deleteAll {
		fmt.Printf("All backups for '%s' deleted successfully\n", instance)
	} else {
		fmt.Printf("Backup '%s' deleted successfully\n", backupName)
	}
	return nil
}
