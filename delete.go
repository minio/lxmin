package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"

	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
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

// Set global states. NOTE: It is deliberately kept monolithic to ensure we dont miss out any flags.
func setGlobalsFromContext(c *cli.Context) error {
	u, err := url.Parse(c.String("endpoint"))
	if err != nil {
		return err
	}

	s3Client, err := minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(c.String("access-key"), c.String("secret-key"), ""),
		Secure: u.Scheme == "https",
	})
	if err != nil {
		return err
	}

	globalS3Clnt = s3Client
	globalBucket = c.String("bucket")
	return nil
}

var deleteCmd = cli.Command{
	Name:    "delete",
	Aliases: []string{"rm"},
	Usage:   "deletes a specific backup by 'name' for an instance from MinIO",
	Action:  deleteMain,
	Before:  setGlobalsFromContext,
	Flags:   append(deleteFlags, globalFlags...),
}

func deleteMain(c *cli.Context) error {
	if len(c.Args()) > 2 {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	deleteAll := c.Bool("all") && c.Bool("force")
	instance := c.Args().Get(0)
	backup := c.Args().Get(1)
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
