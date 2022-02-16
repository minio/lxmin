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

	"github.com/charmbracelet/lipgloss"
	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
)

var infoCmd = cli.Command{
	Name:   "info",
	Usage:  "pretty print tags on an instance image on MinIO",
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
  1. Pretty print tags for a backup 'backup_2022-02-16-04-1040.tar.gz' for instance 'u2':
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

	var table strings.Builder

	list := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(subtle)

	listHeader := lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingRight(1).
		PaddingBottom(1).
		Render

	listItem := lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingRight(1).
		Render

	data := map[string][]string{}
	for k, v := range tags.ToMap() {
		data["Key"] = append(data["Key"], k)
		data["Value"] = append(data["Value"], v)
	}

	items := func(header string) []string {
		var itemRenders []string
		itemRenders = append(itemRenders, listHeader(header))
		for _, d := range data[header] {
			itemRenders = append(itemRenders, listItem(d))
		}
		return itemRenders
	}

	renderLists := []string{}
	for _, header := range []string{"Key", "Value"} {
		renderLists = append(renderLists, list.Render(lipgloss.JoinVertical(lipgloss.Left, items(header)...)))
	}
	lists := lipgloss.JoinHorizontal(lipgloss.Top, renderLists...)
	table.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, lists))

	docStyle := lipgloss.NewStyle().Padding(1, 2, 1, 2)
	fmt.Println(docStyle.Render(table.String()))
	return nil
}
