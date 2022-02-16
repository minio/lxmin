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
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/cheggaaa/pb/v3"
	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/tags"
)

var backupFlags = []cli.Flag{
	cli.BoolFlag{
		Name:  "optimize, O",
		Usage: "use storage driver optimized format",
	},
	cli.StringFlag{
		Name:  "tags",
		Usage: "add additional tags for the backup",
	},
}

var backupCmd = cli.Command{
	Name:   "backup",
	Usage:  "backup an instance image to MinIO",
	Action: backupMain,
	Before: setGlobalsFromContext,
	Flags:  append(backupFlags, globalFlags...),
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [FLAGS] INSTANCENAME

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
EXAMPLES:
  1. Backup an instance 'u2' with storage optimized (faster imports):
     {{.Prompt}} {{.HelpName}} u2 --optimized

  2. Backup an instance 'u2', add custom tags of 'k1=v1&k2=v2' form:
     {{.Prompt}} {{.HelpName}} u2 --optimized --tags "category=prod&project=backup"
`,
}

func backupMain(c *cli.Context) error {
	if len(c.Args()) > 1 {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	tagsHdr := c.String("tags")
	tagsSet, err := tags.Parse(tagsHdr, true)
	if err != nil {
		return err
	}

	instance := strings.TrimSpace(c.Args().Get(0))
	if instance == "" {
		return errors.New("instance name is not optional")
	}

	backup := "backup_" + time.Now().Format("2006-01-02-15-0405") + ".tar.gz"
	cmd := exec.Command("lxc", "export", instance, backup)
	if c.Bool("optimize") {
		cmd = exec.Command("lxc", "export", "--optimized-storage", instance, backup)
	}
	cmd.Stdout = ioutil.Discard
	fmt.Printf("Exporting backup from lxc '%s'... ", backup)
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Println("Done")

	f, err := os.Open(backup)
	if err != nil {
		return err
	}
	defer os.Remove(backup)
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	progress := pb.Start64(fi.Size())
	progress.Set(pb.Bytes, true)
	progress.SetTemplateString(fmt.Sprintf(tmplUp, backup))
	barReader := progress.NewProxyReader(f)
	_, err = globalS3Clnt.PutObject(context.Background(), globalBucket, path.Join(instance, backup), barReader, fi.Size(), minio.PutObjectOptions{
		UserTags: tagsSet.ToMap(),
	})
	barReader.Close()
	return err
}
