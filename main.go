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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/cheggaaa/pb/v3"
	humanize "github.com/dustin/go-humanize"
	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/olekukonko/tablewriter"
)

const (
	tmplUp = `Uploading %s {{ bar . "<" "=" (cycle . "↖" "↗" "↘" "↙" ) "=" ">"}} {{speed . "%%s/s" "? MiB/s" | rndcolor }}`
	tmplDl = `Downloading %s {{ bar . "<" "=" (cycle . "↖" "↗" "↘" "↙" ) "=" ">"}} {{speed . "%%s/s" "? MiB/s" | rndcolor }}`
)

func checkInstance(instance string) error {
	var out bytes.Buffer
	cmd := exec.Command("lxc", "list", instance, "-c", "n", "-f", "csv")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return err
	}
	if strings.TrimSpace(out.String()) == instance {
		return fmt.Errorf("'%s' instance is already running by this name", instance)
	}
	return nil
}

func main() {
	app := cli.NewApp()
	app.Copyright = "MinIO, Inc."
	app.Usage = "backup and restore LXC instances from MinIO"
	app.HideVersion = true
	app.CustomAppHelpTemplate = `NAME:
  {{.Name}} - {{.Usage}}

USAGE:
  {{.Name}} COMMAND INSTANCENAME {{if .VisibleFlags}}[FLAGS]{{end}}

COMMAND:
  backup  backup an instance image to MinIO
  restore restore an instance from MinIO
  list    list all backups from MinIO 
  delete  deletes a specific backup by 'name' for an instance from MinIO

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
`
	app.HideHelpCommand = true
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "endpoint",
			EnvVar: "LXMIN_ENDPOINT",
			Usage:  "endpoint for S3 API call(s)",
		},
		cli.StringFlag{
			Name:   "bucket",
			EnvVar: "LXMIN_BUCKET",
			Usage:  "bucket on MinIO to save/restore backup(s)",
		},
		cli.StringFlag{
			Name:   "access-key",
			EnvVar: "LXMIN_ACCESS_KEY",
			Usage:  "access key credential for S3 API",
		},
		cli.StringFlag{
			Name:   "secret-key",
			EnvVar: "LXMIN_SECRET_KEY",
			Usage:  "secret key credential for S3 API",
		},
		cli.BoolFlag{
			Name:  "all",
			Usage: "delete all backups for an instance, only valid for 'delete' command",
		},
		cli.BoolFlag{
			Name:  "force",
			Usage: "allow all backups to be deleted, only valid when '--all' is specified",
		},
	}

	app.Before = func(c *cli.Context) error {
		if !c.Args().Present() {
			cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
		}
		if len(c.Args()) <= 1 {
			cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
		}
		if c.Bool("help") {
			cli.ShowAppHelpAndExit(c, 0) // last argument is exit code
		}
		commandType := c.Args().Get(0)
		switch commandType {
		case "backup", "list":
			if len(c.Args()) >= 3 {
				cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
			}
		case "delete", "restore":
			if len(c.Args()) >= 4 {
				cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
			}
		}
		_, err := exec.LookPath("lxc")
		return err
	}

	app.Action = func(c *cli.Context) error {
		args := c.Args()
		commandType := args.Get(0)
		instance := args.Get(1)
		backup := args.Get(2)
		if instance == "" {
			return errors.New("instance name is not optional")
		}

		bucket := c.String("bucket")
		if bucket == "" {
			return errors.New("bucket cannot be empty")
		}

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

		var cmd *exec.Cmd
		switch commandType {
		case "backup":
			backup = "backup_" + time.Now().Format("2006-01-02-15-0405") + ".tar.gz"
			cmd = exec.Command("lxc", "export", instance, backup)
			cmd.Stdout = ioutil.Discard
			fmt.Printf("Exporting backup from lxc %s... ", backup)
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
			_, err = s3Client.PutObject(context.Background(), bucket, path.Join(instance, backup), barReader, fi.Size(), minio.PutObjectOptions{})
			barReader.Close()
			if err != nil {
				return err
			}
		case "delete":
			deleteAll := c.Bool("all") && c.Bool("force")
			if backup == "" && !deleteAll {
				return errors.New("backup name is not optional without --all")
			}
			prefix := path.Clean(instance) + "/"
			if backup != "" {
				prefix = path.Join(prefix, backup)
			}
			opts := minio.RemoveObjectOptions{}
			for obj := range s3Client.ListObjects(context.Background(), bucket, minio.ListObjectsOptions{
				Prefix:       prefix,
				WithVersions: true,
			}) {
				if obj.Err != nil {
					return obj.Err
				}
				opts.VersionID = obj.VersionID
				if err := s3Client.RemoveObject(context.Background(), bucket, obj.Key, opts); err != nil {
					return err
				}
			}
			if deleteAll {
				fmt.Printf("All backups for %s deleted successfully\n", instance)
			} else {
				fmt.Printf("Backup %s deleted successfully\n", backup)
			}
		case "list":
			var s strings.Builder
			// Set table header
			table := tablewriter.NewWriter(&s)
			table.SetHeader([]string{"Instance", "Name", "Created", "Size"})
			var data [][]string
			for obj := range s3Client.ListObjects(context.Background(), bucket, minio.ListObjectsOptions{
				Prefix:       path.Clean(instance) + "/",
				WithMetadata: true,
			}) {
				if obj.Err != nil {
					return obj.Err
				}
				data = append(data, []string{
					path.Clean(instance),
					path.Base(obj.Key),
					obj.LastModified.Format(http.TimeFormat),
					humanize.IBytes(uint64(obj.Size))})
			}
			if len(data) > 0 {
				table.AppendBulk(data)
				table.Render()
			}
			fmt.Print(s.String())
		case "restore":
			if backup == "" {
				return errors.New("backup name is not optional")
			}

			if err := checkInstance(instance); err != nil {
				return err
			}

			opts := minio.GetObjectOptions{}
			obj, err := s3Client.GetObject(context.Background(), bucket, path.Join(instance, backup), opts)
			if err != nil {
				return err
			}
			oinfo, err := obj.Stat()
			if err != nil {
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

			cmd = exec.Command("lxc", "import", backup)
			cmd.Stdout = ioutil.Discard
			fmt.Printf("Importing instance %s backup from %s... ", instance, backup)
			if err := cmd.Run(); err != nil {
				return err
			}
			fmt.Print("Done\n")

			fmt.Printf("Starting instance %s... ", instance)
			cmd = exec.Command("lxc", "start", instance)
			cmd.Stdout = ioutil.Discard
			if err := cmd.Run(); err != nil {
				return err
			}
			fmt.Print("Done\n")
		default:
			return fmt.Errorf("command %s is not supported", commandType)
		}
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
