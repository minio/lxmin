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
	"fmt"
	"io/ioutil"
	"mime"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cheggaaa/pb/v3"
	"github.com/dustin/go-humanize"
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
	cli.Int64Flag{
		Name:  "part-size",
		Value: 64 * humanize.MiByte,
		Usage: "configure upload part size per transfer",
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
  3. Backup a remote instance 'u3' on remote 'mylxdserver':
     {{.Prompt}} {{.HelpName}} mylxdserver:u3 --optimized
`,
}

func backupMain(c *cli.Context) error {
	if len(c.Args()) > 1 {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	instance := strings.TrimSpace(c.Args().Get(0))
	if instance == "" {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	partSize := c.Int64("part-size")
	if partSize == 0 {
		partSize = 64 * humanize.MiByte
	}

	tagsHdr := c.String("tags")
	tagsSet, err := tags.Parse(tagsHdr, true)
	if err != nil {
		return err
	}

	backup := "backup_" + time.Now().Format("2006-01-02-15-0405") + ".tar.gz"
	cmd := exec.Command("lxc", "export", instance, backup)
	optimized := c.Bool("optimize")
	if optimized {
		cmd = exec.Command("lxc", "export", "--optimized-storage", instance, backup)
	}
	cmd.Stdout = ioutil.Discard

	p := tea.NewProgram(initSpinnerUI(lxcOpts{
		instance: instance,
		message:  `Preparing backup for (%s) instance: %s`,
	}))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := p.Start(); err != nil {
			os.Exit(1)
		}
	}()

	go func() {
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
		p.Send(true)
	}()

	wg.Wait()

	f, err := os.Open(backup)
	if err != nil {
		return err
	}
	defer os.Remove(backup)
	fi, err := f.Stat()
	if err != nil {
		return err
	}

	usermetadata := map[string]string{}
	// Save additional information if the backup is optimized or not.
	usermetadata["optimized"] = strconv.FormatBool(optimized)
	usermetadata["compressed"] = "true" // This is always true.

	progress := pb.Start64(fi.Size())
	progress.Set(pb.Bytes, true)
	progress.SetTemplateString(fmt.Sprintf(tmplUp, backup))
	barReader := progress.NewProxyReader(f)
	opts := minio.PutObjectOptions{
		UserTags:     tagsSet.ToMap(),
		PartSize:     uint64(partSize),
		UserMetadata: usermetadata,
		ContentType:  mime.TypeByExtension(".tar.gz"),
	}
	_, err = globalS3Clnt.PutObject(context.Background(), globalBucket, path.Join(instance, backup), barReader, fi.Size(), opts)
	barReader.Close()
	return err
}
