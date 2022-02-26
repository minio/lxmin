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
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cheggaaa/pb/v3"
	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
)

var restoreCmd = cli.Command{
	Name:   "restore",
	Usage:  "restore an instance image from MinIO",
	Action: restoreMain,
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
  1. Restore an instance 'u2' from a backup 'backup_2022-02-16-04-1040.tar.gz':
     {{.Prompt}} {{.HelpName}} u2 backup_2022-02-16-04-1040.tar.gz
`,
}

func restoreMain(c *cli.Context) error {
	if len(c.Args()) > 2 {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	instance := strings.TrimSpace(c.Args().Get(0))
	if instance == "" {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	backup := strings.TrimSpace(c.Args().Get(1))
	if backup == "" {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	if err := checkInstance(instance); err != nil {
		return err
	}

	opts := minio.GetObjectOptions{}
	obj, err := globalS3Clnt.GetObject(context.Background(), globalBucket, path.Join(instance, backup), opts)
	if err != nil {
		return err
	}

	oinfo, err := obj.Stat()
	if err != nil {
		obj.Close()
		return err
	}

	progress := pb.Start64(oinfo.Size)
	progress.Set(pb.Bytes, true)
	progress.SetTemplateString(fmt.Sprintf(tmplDl, backup))
	barReader := progress.NewProxyReader(obj)
	w, err := os.Create(backup)
	if err != nil {
		barReader.Close()
		return err
	}
	io.Copy(w, barReader)
	barReader.Close()

	cmd := exec.Command("lxc", "import", backup)
	cmd.Stdout = ioutil.Discard

	p := tea.NewProgram(initSpinnerUI(lxcOpts{
		instance: instance,
		message:  `Launching instance (%s): %s`,
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

		cmd = exec.Command("lxc", "start", instance)
		cmd.Stdout = ioutil.Discard
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}

		p.Send(true)
	}()

	wg.Wait()

	return os.Remove(backup)
}
