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
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Consts for different operations
const (
	Backup  = "backup"
	Restore = "restore"
)

// Consts for backup, restore states
const (
	Failed  = "failed"
	Success = "success"
	Started = "started"
)

type eventInfo struct {
	OpType      string     `json:"opType"`
	State       string     `json:"state"`
	Name        string     `json:"name"`
	Instance    string     `json:"instance"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	FailedAt    *time.Time `json:"failedAt,omitempty"`
	RawURL      string     `json:"rawURL,omitempty"`
	Error       error      `json:"error,omitempty"`
}

func notifyEvent(e eventInfo, endpoint string) {
	data, err := json.Marshal(&e)
	if err != nil {
		log.Println(err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		log.Println(err)
		return
	}

	// Set proper content type.
	req.Header.Set("Content-Type", "application/json")

	resp, err := globalContext.NotifyClnt.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println(fmt.Sprintf("notification endpoint returned error: %s", resp.Status))
		return
	}
}
