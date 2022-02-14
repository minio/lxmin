package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/minio/cli"
	"github.com/olekukonko/tablewriter"
)

type mirrorMessage struct {
	Status     string `json:"status"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	Size       int64  `json:"size"`
	TotalCount int64  `json:"totalCount"`
	TotalSize  int64  `json:"totalSize"`
}

type mirrorStatus struct {
	Status     string `json:"status"`
	Total int64 `json:"total"`
	Transferred int64 `json:"transferred"`
	Speed float64 `json:"speed"`
}

func main() {
	app := cli.NewApp()
	app.Copyright = "MinIO, Inc."
	app.Usage = "backup and restore LXC instances to object storage"
	app.HideVersion = true
	app.CustomAppHelpTemplate = `NAME:
  {{.Name}} - {{.Usage}}

USAGE:
  {{.Name}} COMMAND INSTANCENAME [BACKUPNAME] {{if .VisibleFlags}}[FLAGS]{{end}}

COMMAND:
  create  creates a new backup for an instance
  delete  deletes a specified backup for an instance
  list    lists all the currently captured backups for an instance
  sync    sync all the backups to a central MinIO object store

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
`

	app.HideHelpCommand = true
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "dir",
			Value: "/var/lib/lxc/backups",
			Usage: "specify a custom directory for backups",
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
		if _, err := exec.LookPath("lxc"); err != nil {
			return err
		}
		return os.MkdirAll(c.String("dir"), 0o755)
	}

	app.Action = func(c *cli.Context) error {
		args := c.Args()
		backupDir := c.String("dir")
		commandType := args.Get(0)
		instanceName := args.Get(1)
		backupName := args.Get(2)
		mcAliasName := args.Get(2)
		var cmd *exec.Cmd
		switch commandType {
		case "create":
			cmd = exec.Command("lxc", "list", instanceName)
			var out bytes.Buffer
			cmd.Stdout = &out
			if err := cmd.Run(); err != nil {
				return err
			}
			if err := os.MkdirAll(path.Join(backupDir, instanceName), 0o755); err != nil {
				return err
			}
			if backupName == "" {
				backupName = "backup_" + time.Now().Format("20060102150405")
			}
			cmd = exec.Command("lxc", "export", instanceName, path.Join(backupDir, instanceName, backupName))
			out.Reset()
			cmd.Stdout = &out
			if err := cmd.Run(); err != nil {
				return err
			}
			fmt.Print("%s at %s\n", strings.TrimSpace(out.String()), backupName)
		case "delete":
			if backupName == "" {
				return errors.New("backupName cannot be empty to delete")
			}
			if err := os.Remove(path.Join(backupDir, instanceName, backupName)); err != nil {
				return err
			}
			fmt.Printf("Backup %s deleted successfully\n", backupName)
		case "list":
			var s strings.Builder
			// Set table header
			table := tablewriter.NewWriter(&s)
			table.SetAutoWrapText(false)
			table.SetAutoFormatHeaders(true)
			table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
			table.SetAlignment(tablewriter.ALIGN_LEFT)
			table.SetCenterSeparator("")
			table.SetColumnSeparator("")
			table.SetRowSeparator("")
			table.SetHeaderLine(false)
			table.SetBorder(false)
			table.SetTablePadding("\t") // pad with tabs
			table.SetNoWhiteSpace(true)

			table.SetHeader([]string{"Name", "Created", "Size"})

			dirs, err := os.ReadDir(path.Join(backupDir, instanceName))
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			data := make([][]string, len(dirs))
			for i, dir := range dirs {
				fi, err := dir.Info()
				if err != nil {
					return err
				}
				data[i] = []string{
					dir.Name(),
					fi.ModTime().Format(http.TimeFormat),
					humanize.IBytes(uint64(fi.Size())),
				}
			}
			table.AppendBulk(data)
			table.Render()
			fmt.Print(s.String())
		case "sync":
			if mcAliasName == "" {
				return errors.New("aliasName cannot be empty for 'sync'")
			}
			cmd = exec.Command("lxc", "list", instanceName)
			var out bytes.Buffer
			cmd.Stdout = &out
			if err := cmd.Run(); err != nil {
				return err
			}
			out.Reset()
			cmd = exec.Command("mc", "mirror", "--json", path.Join(backupDir, instanceName), mcAliasName)
			cmd.Stdout = &out
			if err := cmd.Run(); err != nil {
				return err
			}
			scan := bufio.NewScanner(&out)
			for scan.Scan() {
				buf := scan.Bytes()
				var mm mirrorMessage
				if err := json.Unmarshal(buf, &mm); err != nil {
					return err
				}
				if mm.Source != "" {
					fmt.Printf("Backup %s of size %s completed successfully\n", mm.Source, humanize.IBytes(uint64(mm.Size)))
				} else {
					var ms mirrorStatus
					if err := json.Unmarshal(buf, &ms); err != nil {
						return err
					}
					if ms.Speed == 0 {
						fmt.Println("Nothing to backup")
						return nil
					}
					fmt.Printf("Overall transfer speed: %s/s\n", humanize.IBytes(uint64(ms.Speed)))
				}
			}
		}
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
