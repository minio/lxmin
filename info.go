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
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/minio/cli"
)

var infoCmd = cli.Command{
	Name:   "info",
	Usage:  "pretty print tags on an instance image on MinIO",
	Action: infoMain,
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
  1. Pretty print tags for a backup 'backup_2022-02-16-04-1040' for instance 'u2':
     {{.Prompt}} {{.HelpName}} u2 backup_2022-02-16-04-1040
`,
}

func infoMain(c *cli.Context) error {
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

	bkp := backup{instance: instance, backupName: backupName}

	tags, err := globalContext.GetTags(bkp)
	if err != nil {
		return err
	}

	meta, err := globalContext.GetMetadata(bkp)
	if err != nil {
		return err
	}

	var msgBuilder strings.Builder
	// Format properly for alignment based on maxKey leng
	backupName = fmt.Sprintf("%-10s: %s", "Name", backupName)
	msgBuilder.WriteString(backupName + "\n")
	msgBuilder.WriteString(fmt.Sprintf("%-10s: %s ", "Date", meta.LastModified.Format(printDate)) + "\n")
	msgBuilder.WriteString(fmt.Sprintf("%-10s: %-6s ", "Size", humanize.IBytes(uint64(meta.Size))) + "\n")

	maxTagsKey := 0
	for k := range tags.ToMap() {
		if len(k) > maxTagsKey {
			maxTagsKey = len(k)
		}
	}

	maxKeyMetadata := 0
	for k := range meta.UserMetadata {
		if !strings.HasPrefix(strings.ToLower(k), serverEncryptionKeyPrefix) {
			switch k {
			case "Optimized", "Compressed":
				if len(k) > maxKeyMetadata {
					maxKeyMetadata = len(k)
				}
			}
		}
	}

	maxPad := maxTagsKey
	if maxTagsKey < maxKeyMetadata {
		maxPad = maxKeyMetadata
	}

	if maxTagsKey > 0 {
		msgBuilder.WriteString(fmt.Sprintf("%-10s:", "Tags") + "\n")
		for k, v := range tags.ToMap() {
			msgBuilder.WriteString(fmt.Sprintf("  %-*.*s : %s ", maxPad, maxPad, k, v) + "\n")
		}
	}

	if maxKeyMetadata > 0 {
		msgBuilder.WriteString(fmt.Sprintf("%-10s:", "Metadata") + "\n")
		for k, v := range meta.UserMetadata {
			if !strings.HasPrefix(strings.ToLower(k), serverEncryptionKeyPrefix) {
				switch k {
				case "Compressed", "Optimized":
					if v == "true" {
						v = tickCell
					} else {
						v = crossTickCell
					}
				default:
					continue
				}
				msgBuilder.WriteString(fmt.Sprintf("  %-*.*s : %s ", maxKeyMetadata, maxKeyMetadata, k, v) + "\n")
			}
		}
	}

	fmt.Println(msgBuilder.String())
	return nil
}
