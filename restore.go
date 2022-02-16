package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

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
}

func restoreMain(c *cli.Context) error {
	if len(c.Args()) > 2 {
		cli.ShowAppHelpAndExit(c, 1) // last argument is exit code
	}

	instance := c.Args().Get(0)
	backup := c.Args().Get(1)

	if backup == "" {
		return errors.New("backup name is not optional")
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
	return nil
}
