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
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
	"github.com/olekukonko/tablewriter"
)

var infoCmd = cli.Command{
	Name:   "info",
	Usage:  "display metadata of an instance image on MinIO",
	Action: infoMain,
	Before: setGlobalsFromContext,
	Flags:  globalFlags,
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [FLAGS] INSTANCENAME BACKUPNAME

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
EXAMPLES:
  1. Pretty print tags and additional metadata about a backup 'backup_2022-02-16-04-1040.tar.gz' for instance 'u2':
     {{.Prompt}} {{.HelpName}} u2 backup_2022-02-16-04-1040.tar.gz
`,
}

func infoMain(c *cli.Context) error {
	if len(c.Args()) > 2 {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	instance := strings.TrimSpace(c.Args().Get(0))
	backup := strings.TrimSpace(c.Args().Get(1))

	if backup == "" {
		return errors.New("backup name is not optional")
	}

	tags, err := globalS3Clnt.GetObjectTagging(context.Background(), globalBucket, path.Join(instance, backup), minio.GetObjectTaggingOptions{})
	if err != nil {
		return err
	}

	var s strings.Builder
	// Set table header
	table := tablewriter.NewWriter(&s)
	table.SetHeader([]string{"Key", "Value"})
	var data [][]string
	for k, v := range tags.ToMap() {
		data = append(data, []string{
			k,
			v,
		})
	}
	if len(data) > 0 {
		table.AppendBulk(data)
	}
	table.Render()
	fmt.Print(s.String())
	return nil
}
