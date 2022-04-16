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
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cheggaaa/pb/v3"
	"github.com/minio/cli"
	"github.com/minio/minio-go/v7/pkg/set"
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
  1. Restore an instance 'u2' from a backup 'backup_2022-02-16-04-1040':
     {{.Prompt}} {{.HelpName}} u2 backup_2022-02-16-04-1040
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

	backupName := strings.TrimSpace(c.Args().Get(1))
	if backupName == "" {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	if err := checkInstance(instance); err != nil {
		return err
	}

	bkp := backup{instance: instance, backupName: backupName}

	// List and collect all backup related files.
	resInfo := collectBackupInfo(globalContext, bkp)

	// Download all backup files to staging directory
	err := downloadBackupFiles(globalContext, bkp, resInfo)
	if err != nil {
		return err
	}

	restoreProfiles(globalContext, instance, backupName, resInfo)

	restoreInstanceCLI(globalContext, bkp)

	return nil
}

func restoreInstanceCLI(ctx *lxminContext, bkp backup) {
	var lastCmd []string
	var outBuf *bytes.Buffer
	restoreCmd := func() tea.Msg {
		ob, err := restoreInstance(ctx, bkp)
		if err != nil {
			outBuf = ob
			return err
		}
		return true
	}

	sUI := initCmdSpinnerUI(
		restoreCmd,
		cOpts{instance: bkp.instance, message: `%s Launching instance: %s`},
	)
	if err := tea.NewProgram(sUI).Start(); err != nil {
		log.Printf("Last command: `%s`", strings.Join(lastCmd, " "))
		log.Printf("Output: %s", string(outBuf.Bytes()))
		log.Fatalln(err)
	}
}

func restoreProfiles(ctx *lxminContext, instance, backupNamePrefix string, resInfo restoreInfo) {
	existingProfiles := set.NewStringSet()
	retrieveExistingProfiles := func() tea.Msg {
		p, err := fetchExistingProfiles()
		if err != nil {
			return err
		}
		existingProfiles = p
		return true
	}

	sUI := initCmdSpinnerUI(
		retrieveExistingProfiles,
		cOpts{instance: instance, message: `%s Retrieving existing profiles list: %s`},
	)
	if err := tea.NewProgram(sUI).Start(); err != nil {
		log.Fatalln(err)
	}

	for i, pf := range resInfo.profiles {
		restoreProfile := func() tea.Msg {
			err := restoreProfile(ctx, pf, resInfo.profileKeys[i], existingProfiles)
			if w, ok := err.(warnMsgErr); ok {
				return w.msg
			} else if err != nil {
				return err
			}
			return true
		}

		sUI := initCmdSpinnerUI(restoreProfile,
			cOpts{instance: instance, message: `%s Created profile ` + pf + ` for: %s`})
		if err := tea.NewProgram(sUI).Start(); err != nil {
			log.Fatalln(err)
		}
	}
}

func downloadBackupFiles(ctx *lxminContext, bkp backup, resInfo restoreInfo) error {
	bar := pb.Start64(resInfo.totalSize)
	bar.Set(pb.Bytes, true)
	defer bar.Finish()

	// Download profiles
	for _, pkey := range resInfo.profileKeys {
		err := ctx.downloadItem(pkey, bar)
		if err != nil {
			return fmt.Errorf("Error downloading profile file %s: %v", pkey, err)
		}
	}

	// Download instance backup
	if err := ctx.downloadItem(bkp.key(), bar); err != nil {
		return fmt.Errorf("Error downloading instance backup %s: %v", bkp.key(), err)
	}
	return nil
}

type barUpdateWriter struct {
	w   io.Writer
	bar *pb.ProgressBar
}

func newBarUpdateWriter(fpath string, bar *pb.ProgressBar, tmpl string) (*barUpdateWriter, error) {
	w, err := os.Create(fpath)
	if err != nil {
		return nil, fmt.Errorf("Unable to create %s: %v", fpath, err)
	}

	bar.SetTemplateString(fmt.Sprintf(tmpl, path.Base(fpath)))

	return &barUpdateWriter{
		w:   w,
		bar: bar,
	}, nil
}

func (b *barUpdateWriter) Write(p []byte) (n int, err error) {
	n, err = b.w.Write(p)
	b.bar.Add(n)
	return
}

// Close closes the underlying writer if it is a io.Closer.
func (b *barUpdateWriter) Close() error {
	if c, ok := b.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// collectBackupInfo collects backup info so we can show a progress bar and
// restore profiles in order.
func collectBackupInfo(ctx *lxminContext, bkp backup) (bi restoreInfo) {
	populateRestoreInfo := func() tea.Msg {
		ri, err := ctx.fetchRestoreInfo(bkp)
		if err != nil {
			return err
		}
		bi = ri
		return true
	}

	sUI := initCmdSpinnerUI(
		populateRestoreInfo,
		cOpts{instance: bkp.instance, message: `%s Collecting info for backup: %s`},
	)
	if err := tea.NewProgram(sUI).Start(); err != nil {
		log.Fatalln(err)
	}

	return bi
}
