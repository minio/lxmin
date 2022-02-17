package main

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
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

var (
	globalS3Clnt *minio.Client
	globalBucket string
)

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
  1. Delete a backup 'backup_2022-02-16-04-1040.tar.gz' for instance 'u2':
     {{.Prompt}} {{.HelpName}} u2 backup_2022-02-16-04-1040.tar.gz
`,
}

func deleteMain(c *cli.Context) error {
	if len(c.Args()) > 2 {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	instance := strings.TrimSpace(c.Args().Get(0))
	if instance == "" {
		return errors.New("instance name is not optional")
	}

	backup := strings.TrimSpace(c.Args().Get(1))
	deleteAll := c.Bool("all") && c.Bool("force")
	if backup == "" && !deleteAll {
		return errors.New("backup name is not optional without --all")
	}
	prefix := path.Clean(instance) + "/"
	if backup != "" {
		prefix = path.Join(prefix, backup)
	}
	opts := minio.RemoveObjectOptions{}
	for obj := range globalS3Clnt.ListObjects(context.Background(), globalBucket, minio.ListObjectsOptions{
		Prefix:       prefix,
		WithVersions: true,
	}) {
		if obj.Err != nil {
			return obj.Err
		}
		opts.VersionID = obj.VersionID
		if err := globalS3Clnt.RemoveObject(context.Background(), globalBucket, obj.Key, opts); err != nil {
			return err
		}
	}
	if deleteAll {
		fmt.Printf("All backups for '%s' deleted successfully\n", instance)
	} else {
		fmt.Printf("Backup '%s' deleted successfully\n", backup)
	}
	return nil
}
