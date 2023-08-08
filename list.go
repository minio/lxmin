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
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/minio/cli"
)

var listCmd = cli.Command{
	Name:    "list",
	Aliases: []string{"ls"},
	Usage:   "list all backups from MinIO",
	Action:  listMain,
	Before:  setGlobalsFromContext,
	Flags:   globalFlags,
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [FLAGS] [INSTANCENAME]

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
EXAMPLES:
  1. List all backups:
     {{.Prompt}} {{.HelpName}}
  2. List all backups by instance name 'u2':
     {{.Prompt}} {{.HelpName}} u2
`,
}

const (
	tickCell      string = "✔ "
	crossTickCell string = "✗ "
)

var subtle = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}

func listMain(c *cli.Context) error {
	if len(c.Args()) > 1 {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	instance := strings.TrimSpace(c.Args().Get(0))

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

	backups, err := globalContext.ListBackups(instance)
	if err != nil {
		return err
	}

	data := map[string][]string{}
	for _, bkp := range backups {
		data["Instance"] = append(data["Instance"], bkp.Instance)
		data["Name"] = append(data["Name"], bkp.Name)
		data["Created"] = append(data["Created"], bkp.Created.Format(printDate))
		data["Size"] = append(data["Size"], humanize.IBytes(uint64(bkp.Size)))
		if *bkp.Optimized {
			data["Optimized"] = append(data["Optimized"], tickCell)
		} else {
			data["Optimized"] = append(data["Optimized"], crossTickCell)
		}
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
	for _, header := range []string{"Instance", "Name", "Created", "Size", "Optimized"} {
		renderLists = append(renderLists, list.Render(lipgloss.JoinVertical(lipgloss.Left, items(header)...)))
	}
	lists := lipgloss.JoinHorizontal(lipgloss.Top, renderLists...)
	table.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, lists))

	docStyle := lipgloss.NewStyle()
	fmt.Println(docStyle.Render(table.String()))
	return nil
}
