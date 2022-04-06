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
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cheggaaa/pb/v3"
	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/set"
	"gopkg.in/yaml.v2"
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

	backup := strings.TrimSpace(c.Args().Get(1))
	if backup == "" {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	if err := checkInstance(instance); err != nil {
		return err
	}

	// List and collect all backup related files.
	resInfo := collectBackupInfo(globalContext, instance, backup)

	// Download all backup files to staging directory
	err := downloadBackupFiles(globalContext, instance, backup, resInfo)
	if err != nil {
		return err
	}

	restoreProfiles(globalContext, instance, backup, resInfo)

	restoreInstance(globalContext, instance, backup)

	return nil
}

func restoreInstance(ctx *lxminContext, instance, backupNamePrefix string) {
	var lastCmd []string
	var outBuf bytes.Buffer
	restoreCmd := func() tea.Msg {
		localPath := path.Join(ctx.StagingRoot, backupNamePrefix+"_instance.tar.gz")

		lastCmd = []string{"lxc", "import", localPath}
		cmd := exec.Command(lastCmd[0], lastCmd[1:]...)
		cmd.Stdout = ioutil.Discard
		cmd.Stderr = &outBuf
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("Error importing instance: %v", err)
		}

		// Clear outBuf for next command
		outBuf = bytes.Buffer{}
		lastCmd = []string{"lxc", "start", instance}
		cmd = exec.Command(lastCmd[0], lastCmd[1:]...)
		cmd.Stdout = ioutil.Discard
		cmd.Stderr = &outBuf
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("Error starting instance: %v", err)
		}

		defer os.Remove(localPath)
		return true
	}

	sUI := initCmdSpinnerUI(
		restoreCmd,
		cOpts{instance: instance, message: `%s Launching instance: %s`},
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
		// First get the list of existing profiles, so we can restore
		// only missing ones.
		var outBuf bytes.Buffer
		cmd := exec.Command("lxc", "profile", "list", "-f", "yaml")
		cmd.Stdout = &outBuf

		if err := cmd.Run(); err != nil {
			return err
		}

		type profileInfo struct {
			Name string `yaml:"name"`
		}

		var profileInfos []profileInfo
		if err := yaml.Unmarshal(outBuf.Bytes(), &profileInfos); err != nil {
			return fmt.Errorf("Unable to parse profiles list: %v", err)
		}

		for _, pi := range profileInfos {
			existingProfiles.Add(pi.Name)
		}
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
			proPath := path.Join(ctx.StagingRoot, path.Base(resInfo.profileKeys[i]))

			if existingProfiles.Contains(pf) {
				defer os.Remove(proPath)
				return warningMessage{
					msg: `%s Skipping profile ` + pf + ` as it already exists for: %s`,
				}
			}

			cmd := exec.Command("lxc", "profile", "create", pf)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("Error creating profile %s: %v", pf, err)
			}

			profileFile, err := os.Open(proPath)
			if err != nil {
				return fmt.Errorf("Error opening backup file %s: %v", proPath, err)
			}

			cmd = exec.Command("lxc", "profile", "edit", pf)
			cmd.Stdin = profileFile
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("Error restoring profile %s: %v", pf, err)
			}

			defer os.Remove(proPath)
			return true
		}

		sUI := initCmdSpinnerUI(restoreProfile,
			cOpts{instance: instance, message: `%s Created profile ` + pf + ` for: %s`})
		if err := tea.NewProgram(sUI).Start(); err != nil {
			log.Fatalln(err)
		}
	}
}

func downloadBackupFiles(ctx *lxminContext, instance, backupNamePrefix string, resInfo restoreInfo) error {
	bar := pb.Start64(resInfo.totalSize)
	bar.Set(pb.Bytes, true)
	defer bar.Finish()

	downloadFn := func(objPath string) error {
		fpath := path.Join(ctx.StagingRoot, path.Base(objPath))
		barWriter, err := newBarUpdateWriter(fpath, bar, tmplDl)
		if err != nil {
			return err
		}
		defer barWriter.Close()

		obj, err := ctx.Clnt.GetObject(context.Background(), ctx.Bucket, objPath, minio.GetObjectOptions{})
		if err != nil {
			return err
		}
		defer obj.Close()

		_, err = io.Copy(barWriter, obj)
		return err
	}

	// Download profiles
	for _, pkey := range resInfo.profileKeys {
		err := downloadFn(pkey)
		if err != nil {
			return fmt.Errorf("Error downloading profile file %s: %v", pkey, err)
		}
	}

	// Download instance backup
	instanceBackupKey := path.Join(instance, backupNamePrefix+"_instance.tar.gz")
	if err := downloadFn(instanceBackupKey); err != nil {
		return fmt.Errorf("Error downloading instance backup %s: %v", instanceBackupKey, err)
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

type restoreInfo struct {
	profiles    []string
	profileKeys []string
	totalSize   int64
}

// collectBackupInfo collects backup info so we can show a progress bar and
// restore profiles in order.
func collectBackupInfo(ctx *lxminContext, instance, backupNamePrefix string) (bi restoreInfo) {
	populateRestoreInfo := func() tea.Msg {
		pno := 0
		for obj := range ctx.Clnt.ListObjects(context.Background(), ctx.Bucket, minio.ListObjectsOptions{
			WithMetadata: false,
			Prefix:       path.Join(instance, backupNamePrefix+"_profile_"),
			Recursive:    true,
		}) {
			if obj.Err != nil {
				return fmt.Errorf("Error listing backup files: %v", obj.Err)
			}

			expectedProfilePrefix := fmt.Sprintf("%s_profile_%03d_", backupNamePrefix, pno)
			pno += 1
			profileName := strings.TrimPrefix(
				strings.TrimSuffix(path.Base(obj.Key), ".yaml"),
				expectedProfilePrefix,
			)

			// Validate the profile object name.
			if !strings.HasPrefix(path.Base(obj.Key), expectedProfilePrefix) || !strings.HasSuffix(obj.Key, ".yaml") || profileName == "" {
				return fmt.Errorf("Unexpected profile file found: %s", obj.Key)
			}

			bi.totalSize += obj.Size
			bi.profiles = append(bi.profiles, profileName)
			bi.profileKeys = append(bi.profileKeys, obj.Key)
		}

		instanceBackupName := backupNamePrefix + "_instance.tar.gz"
		oi, err := ctx.Clnt.StatObject(context.Background(), ctx.Bucket, path.Join(instance, instanceBackupName), minio.StatObjectOptions{})
		if err != nil {
			return fmt.Errorf("Error getting instance backup file info: %v", err)
		}

		bi.totalSize += oi.Size
		return true
	}

	sUI := initCmdSpinnerUI(
		populateRestoreInfo,
		cOpts{instance: instance, message: `%s Collecting info for backup: %s`},
	)
	if err := tea.NewProgram(sUI).Start(); err != nil {
		log.Fatalln(err)
	}

	return bi
}
