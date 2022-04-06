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
	"gopkg.in/yaml.v2"
)

var backupFlags = []cli.Flag{
	cli.BoolFlag{
		Name:  "optimized, O",
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

	if err := checkInstance(instance); err == nil {
		return fmt.Errorf("no instance found by name: '%s'", instance)
	}

	backupNamePrefix := "backup_" + time.Now().Format("2006-01-02-15-0405")

	// Save profiles to files.
	profiles, profileInfo, err := backupProfiles(globalContext, instance, backupNamePrefix)
	if err != nil {
		return err
	}

	instanceBackupName, instanceBackupSize, err := backupInstance(globalContext, c.Bool("optimized"), instance, backupNamePrefix)
	if err != nil {
		return err
	}

	// Backup to MinIO

	// Collect total upload size.
	var totalSize int64
	backupPath := path.Join(globalContext.StagingRoot, instanceBackupName)
	if st, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("Unable to stat file %s: %v", backupPath, err)
	} else {
		totalSize = st.Size()
	}
	for _, v := range profileInfo {
		totalSize += v.Size
	}

	progress := pb.Start64(totalSize)
	progress.Set(pb.Bytes, true)

	if err := uploadInstanceBackup(globalContext, c.Bool("optimized"), instance, instanceBackupName, instanceBackupSize, progress, tagsSet, partSize); err != nil {
		return err
	}
	if err := uploadProfilesBackup(globalContext, instance, profiles, profileInfo, progress, tagsSet, partSize); err != nil {
		return err
	}

	progress.Finish()
	return err
}

func uploadInstanceBackup(ctx *lxminContext, optimized bool, instance, backupName string, size int64, bar *pb.ProgressBar, tagsSet *tags.Tags, partSize int64) error {
	usermetadata := map[string]string{}
	// Save additional information if the backup is optimized or not.
	usermetadata["optimized"] = strconv.FormatBool(optimized)
	usermetadata["compressed"] = "true" // This is always true.

	fpath := path.Join(ctx.StagingRoot, backupName)
	barReader, err := newBarUpdateReader(fpath, bar, tmplUp)
	if err != nil {
		return err
	}

	defer barReader.Close()
	defer os.Remove(fpath)
	opts := minio.PutObjectOptions{
		UserTags:     tagsSet.ToMap(),
		PartSize:     uint64(partSize),
		UserMetadata: usermetadata,
		ContentType:  mime.TypeByExtension(".tar.gz"),
	}
	_, err = globalContext.Clnt.PutObject(context.Background(), globalContext.Bucket, path.Join(instance, backupName), barReader, size, opts)
	if err != nil {
		return fmt.Errorf("Error uploading file %s: %v", fpath, err)
	}
	return nil
}

func uploadProfilesBackup(ctx *lxminContext, instance string, pList []string, prInfo map[string]profileInfo, bar *pb.ProgressBar, tagsSet *tags.Tags, partSize int64) error {
	for _, profile := range pList {
		err := func() error {
			profileFile := prInfo[profile].FileName
			size := prInfo[profile].Size
			fpath := path.Join(ctx.StagingRoot, profileFile)
			barReader, err := newBarUpdateReader(fpath, bar, tmplUp)
			if err != nil {
				return err
			}
			defer barReader.Close()
			defer os.Remove(fpath)

			opts := minio.PutObjectOptions{
				UserTags:    tagsSet.ToMap(),
				PartSize:    uint64(partSize),
				ContentType: mime.TypeByExtension(".yaml"),
			}
			_, err = ctx.Clnt.PutObject(context.Background(), ctx.Bucket, path.Join(instance, profileFile), barReader, size, opts)
			if err != nil {
				return fmt.Errorf("Error uploading file %s: %v", fpath, err)
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

type barUpdateReader struct {
	r   io.Reader
	bar *pb.ProgressBar
}

func newBarUpdateReader(fpath string, bar *pb.ProgressBar, tmpl string) (*barUpdateReader, error) {
	r, err := os.Open(fpath)
	if err != nil {
		return nil, fmt.Errorf("Unable to open %s: %v", fpath, err)
	}

	bar.SetTemplateString(fmt.Sprintf(tmpl, path.Base(fpath)))

	return &barUpdateReader{
		r:   r,
		bar: bar,
	}, nil
}

func (b *barUpdateReader) Read(p []byte) (n int, err error) {
	n, err = b.r.Read(p)
	b.bar.Add(n)
	return
}

// Close closes the underlying reader if it is a io.Closer.
func (b *barUpdateReader) Close() error {
	if c, ok := b.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func backupInstance(ctx *lxminContext, optimized bool, instance, backupNamePrefix string) (string, int64, error) {
	backup := backupNamePrefix + "_instance.tar.gz"
	localPath := path.Join(ctx.StagingRoot, backup)

	cmd := exec.Command("lxc", "export", instance, localPath)
	if optimized {
		cmd = exec.Command("lxc", "export", "--optimized-storage", instance, localPath)
	}
	cmd.Stdout = ioutil.Discard

	p := tea.NewProgram(initSpinnerUI(lxcOpts{
		instance: instance,
		message:  `%s Preparing backup for instance: %s`,
	}))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := cmd.Run(); err != nil {
			p.Send(err)
			log.Fatalln(err)
		}
		p.Send(true)
	}()

	go func() {
		if err := p.Start(); err != nil {
			log.Fatalln(err)
		}
	}()

	wg.Wait()

	if s, err := os.Stat(localPath); err != nil {
		return "", 0, fmt.Errorf("Unable to stat file %s: %v", localPath, err)
	} else {
		return backup, s.Size(), nil
	}
}

func listProfiles(ctx *lxminContext, instance string) ([]string, error) {
	var outBuf bytes.Buffer
	cmd := exec.Command("lxc", "config", "show", instance)
	cmd.Stdout = &outBuf

	p := tea.NewProgram(initSpinnerUI(lxcOpts{
		instance: instance,
		message:  `%s Listing profiles for instance: %s`,
	}))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := cmd.Run(); err != nil {
			p.Send(err)
			log.Fatalln(err)
		}
		p.Send(true)
	}()

	go func() {
		if err := p.Start(); err != nil {
			log.Fatalln(err)
		}
	}()

	wg.Wait()

	type profileInfo struct {
		Profiles []string `yaml:"profiles"`
	}

	var profiles profileInfo
	if err := yaml.Unmarshal(outBuf.Bytes(), &profiles); err != nil {
		return nil, fmt.Errorf("Unable to parse profiles list: %v", err)
	}
	return profiles.Profiles, nil
}

type profileInfo struct {
	FileName string
	Size     int64
}

func backupProfiles(ctx *lxminContext, instance, backupNamePrefix string) ([]string, map[string]profileInfo, error) {
	profiles, err := listProfiles(ctx, instance)
	if err != nil {
		return nil, nil, err
	}

	if len(profiles) > 1000 {
		log.Fatalf("More than a 1000 profiles per instance not supported.")
	}

	pInfo := make(map[string]profileInfo, len(profiles))
	for pno, profile := range profiles {
		// Profiles are numbered because their order matters - settings
		// in the later profiles override those from earlier profiles.
		profileFile := fmt.Sprintf("%s_profile_%03d_%s.yaml", backupNamePrefix, pno, profile)
		profilePath := path.Join(ctx.StagingRoot, profileFile)
		pf, err := os.Create(profilePath)
		if err != nil {
			return nil, nil, fmt.Errorf("Unable to create backup file %s: %v", profileFile, err)
		}
		cmd := exec.Command("lxc", "profile", "show", profile)
		cmd.Stdout = pf

		p := tea.NewProgram(initSpinnerUI(lxcOpts{
			instance: instance,
			message:  `%s Fetching profile '` + profile + `' for instance: %s`,
		}))

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := cmd.Run(); err != nil {
				p.Send(err)
				log.Fatalln(err)
			}
			p.Send(true)
		}()

		go func() {
			if err := p.Start(); err != nil {
				log.Fatalln(err)
			}
		}()

		wg.Wait()

		// Sync file to disk
		if err := pf.Sync(); err != nil {
			return nil, nil, fmt.Errorf("Error syncing profile file %s to disk: %v", profileFile, err)
		}

		// Save size of the file for showing progress later.
		if stat, err := pf.Stat(); err != nil {
			return nil, nil, fmt.Errorf("Unable to stat file %s: %v", profileFile, err)
		} else {
			pInfo[profile] = profileInfo{
				FileName: profileFile,
				Size:     stat.Size(),
			}
		}

		// Close the file
		if err := pf.Close(); err != nil {
			return nil, nil, fmt.Errorf("Unable to close file %s: %v", profileFile, err)
		}
	}

	return profiles, pInfo, nil
}
