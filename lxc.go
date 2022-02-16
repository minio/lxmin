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
	"fmt"
	"os/exec"
	"strings"
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
