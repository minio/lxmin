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
	"sync"

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
	p := tea.NewProgram(initSpinnerUI(lxcOpts{
		instance: instance,
		message:  `%s Launching instance: %s`,
	}))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := p.Start(); err != nil {
			log.Fatalln(err)
		}
	}()

	localPath := path.Join(ctx.StagingRoot, backupNamePrefix+"_instance.tar.gz")
	go func() {
		{
			var outBuf bytes.Buffer
			cmd := exec.Command("lxc", "import", localPath)
			cmd.Stdout = ioutil.Discard
			cmd.Stderr = &outBuf
			if err := cmd.Run(); err != nil {
				err := fmt.Errorf("Error importing instance: %v", err)
				p.Send(err)
				log.Printf("lxc import output:\n%s\n", string(outBuf.Bytes()))
				log.Fatalln(err)
			}
		}

		var outBuf bytes.Buffer
		cmd := exec.Command("lxc", "start", instance)
		cmd.Stdout = ioutil.Discard
		cmd.Stderr = &outBuf
		if err := cmd.Run(); err != nil {
			err := fmt.Errorf("Error starting instance: %v", err)
			p.Send(err)
			log.Printf("lxc start output:\n%s\n", string(outBuf.Bytes()))
			log.Fatalln(err)
		}

		p.Send(true)
	}()

	wg.Wait()

	os.Remove(localPath)
}

func restoreProfiles(ctx *lxminContext, instance, backupNamePrefix string, resInfo restoreInfo) {
	p := tea.NewProgram(initSpinnerUI(lxcOpts{
		instance: instance,
		message:  `%s Restoring profiles for instance: %s`,
	}))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		// First get the list of existing profiles, so we can restore
		// only missing ones.
		var outBuf bytes.Buffer
		cmd := exec.Command("lxc", "profile", "list", "-f", "yaml")
		cmd.Stdout = &outBuf

		if err := cmd.Run(); err != nil {
			p.Send(err)
			log.Fatalln(err)
		}

		type profileInfo struct {
			Name string `yaml:"name"`
		}

		var profileInfos []profileInfo
		if err := yaml.Unmarshal(outBuf.Bytes(), &profileInfos); err != nil {
			err = fmt.Errorf("Unable to parse profiles: %v", err)
			p.Send(err)
			log.Fatalln(err)
		}

		profiles := set.NewStringSet()
		for _, pi := range profileInfos {
			profiles.Add(pi.Name)
		}

		// Restore missing profiles only.
		for _, pf := range resInfo.profiles {
			if profiles.Contains(pf) {
				log.Printf("Skipping profile %s as it already exists.", pf)
				continue
			}

			errExit := func(err error) {
				p.Send(err)
				log.Fatalln(err)
			}

			cmd := exec.Command("lxc", "profile", "create", pf)
			if err := cmd.Run(); err != nil {
				errExit(err)
			}

			proPath := path.Join(ctx.StagingRoot, pf)
			profileFile, err := os.Open(proPath)
			if err != nil {
				errExit(err)
			}

			cmd = exec.Command("lxc", "profile", "edit", pf)
			cmd.Stdin = profileFile
			if err := cmd.Run(); err != nil {
				errExit(err)
			}

			log.Printf("Created profile %s from backup", pf)
		}

		p.Send(true)
	}()

	go func() {
		if err := p.Start(); err != nil {
			log.Fatalln(err)
		}
	}()

	wg.Wait()

	for _, pf := range resInfo.profiles {
		os.Remove(path.Join(ctx.StagingRoot, pf))
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
	p := tea.NewProgram(initSpinnerUI(lxcOpts{
		instance: instance,
		message:  `%s Collecting info for backup: %s`,
	}))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pno := 0
		for obj := range ctx.Clnt.ListObjects(context.Background(), ctx.Bucket, minio.ListObjectsOptions{
			WithMetadata: false,
			Prefix:       path.Join(instance, backupNamePrefix+"_profile_"),
			Recursive:    true,
		}) {
			if obj.Err != nil {
				err := fmt.Errorf("Error listing backup files: %v", obj.Err)
				p.Send(err)
				log.Fatalln(err)
			}

			expectedProfilePrefix := fmt.Sprintf("%s_profile_%03d_", backupNamePrefix, pno)
			pno += 1
			profileName := strings.TrimPrefix(
				strings.TrimSuffix(path.Base(obj.Key), ".yaml"),
				expectedProfilePrefix,
			)

			// Validate the profile object name.
			if !strings.HasPrefix(path.Base(obj.Key), expectedProfilePrefix) || !strings.HasSuffix(obj.Key, ".yaml") || profileName == "" {
				err := fmt.Errorf("Unexpected profile file found: %s", obj.Key)
				p.Send(err)
				log.Fatalln(err)
			}

			bi.totalSize += obj.Size
			bi.profiles = append(bi.profiles, profileName)
			bi.profileKeys = append(bi.profileKeys, obj.Key)
		}

		instanceBackupName := backupNamePrefix + "_instance.tar.gz"
		oi, err := ctx.Clnt.StatObject(context.Background(), ctx.Bucket, path.Join(instance, instanceBackupName), minio.StatObjectOptions{})
		if err != nil {
			err := fmt.Errorf("Error getting instance backup file info: %v", err)
			p.Send(err)
			log.Fatalln(err)
		}

		bi.totalSize += oi.Size
		p.Send(true)
	}()

	go func() {
		if err := p.Start(); err != nil {
			log.Fatalln(err)
		}
	}()

	wg.Wait()

	return bi
}
