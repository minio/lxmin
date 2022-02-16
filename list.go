package main

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
	"github.com/olekukonko/tablewriter"
)

var listCmd = cli.Command{
	Name:    "list",
	Aliases: []string{"ls"},
	Usage:   "list all backups from MinIO",
	Action:  listMain,
	Before:  setGlobalsFromContext,
	Flags:   globalFlags,
}

func listMain(c *cli.Context) error {
	if len(c.Args()) > 1 {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	instance := c.Args().Get(0)
	if instance != "" {
		instance = path.Clean(instance) + "/"
	}

	var s strings.Builder
	// Set table header
	table := tablewriter.NewWriter(&s)
	table.SetHeader([]string{"Instance", "Name", "Created", "Size"})
	var data [][]string
	for obj := range globalS3Clnt.ListObjects(context.Background(), globalBucket, minio.ListObjectsOptions{
		Prefix:       instance,
		WithMetadata: true,
	}) {
		if obj.Err != nil {
			return obj.Err
		}
		inst := path.Clean(instance)
		if inst == "" {
			inst = path.Dir(obj.Key)
		}
		data = append(data, []string{
			inst,
			path.Base(obj.Key),
			obj.LastModified.Format(http.TimeFormat),
			humanize.IBytes(uint64(obj.Size))})
	}
	if len(data) > 0 {
		table.AppendBulk(data)
		table.Render()
	}
	fmt.Print(s.String())
	return nil
}
