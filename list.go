package main

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
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

var (
	subtle = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
)

func listMain(c *cli.Context) error {
	if len(c.Args()) > 1 {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	instance := strings.TrimSpace(c.Args().Get(0))
	if instance != "" {
		instance = path.Clean(instance) + "/"
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
	for obj := range globalS3Clnt.ListObjects(context.Background(), globalBucket, minio.ListObjectsOptions{
		Prefix:       instance,
		Recursive:    true,
		WithMetadata: true,
	}) {
		if obj.Err != nil {
			return obj.Err
		}
		inst := path.Clean(instance)
		if inst == "" || inst == "." {
			inst = path.Dir(obj.Key)
		}
		data["Instance"] = append(data["Instance"], inst)
		data["Name"] = append(data["Name"], path.Base(obj.Key))
		data["Created"] = append(data["Created"], obj.LastModified.Format(http.TimeFormat))
		data["Size"] = append(data["Size"], humanize.IBytes(uint64(obj.Size)))
		if _, ok := obj.UserMetadata["X-Amz-Meta-Optimized"]; ok {
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

	docStyle := lipgloss.NewStyle().Padding(1, 2, 1, 2)
	fmt.Println(docStyle.Render(table.String()))
	return nil
}
