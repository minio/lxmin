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
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	printDate                 = "2006-01-02 15:04:05 MST"
	serverEncryptionKeyPrefix = "x-amz-server-side-encryption"
)

type spinnerUI struct {
	spinner  spinner.Model
	quitting bool
	err      error
	opts     lxcOpts
}

func (m *spinnerUI) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *spinnerUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.err = errors.New("canceling")
			m.quitting = true
			return m, tea.Quit
		default:
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case error:
		m.err = msg
		m.quitting = true
		return m, tea.Quit
	case bool:
		if msg {
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *spinnerUI) View() string {
	spin := m.spinner.View()
	if m.quitting {
		if m.err != nil {
			return m.err.Error()
		}
		spin = "✔"
		m.opts.message += "\n"
	}

	return fmt.Sprintf(m.opts.message, spin, m.opts.instance)
}

type lxcOpts struct {
	instance, message string
}

func initSpinnerUI(opts lxcOpts) *spinnerUI {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return &spinnerUI{
		spinner: s,
		opts:    opts,
	}
}

var instanceExists = errors.New("instance exists")

func checkInstance(instance string) error {
	var out bytes.Buffer
	cmd := exec.Command("lxc", "list", instance, "-c", "n", "-f", "csv")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return err
	}
	if strings.TrimSpace(out.String()) == instance {
		return fmt.Errorf("'%s' instance is already running by this name: %w", instance, instanceExists)
	}
	return nil
}
