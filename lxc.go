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
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/minio/minio-go/v7/pkg/set"
	"gopkg.in/yaml.v2"
)

const (
	printDate                 = "2006-01-02 15:04:05 MST"
	serverEncryptionKeyPrefix = "x-amz-server-side-encryption"
)

type cmdSpinnerUI struct {
	spinner    spinner.Model
	quitting   bool
	err        error
	warningMsg string
	cmdFn      func() tea.Msg
	opts       cOpts
}

type cOpts struct {
	instance, message string
}

func (m *cmdSpinnerUI) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.cmdFn)
}

func (m *cmdSpinnerUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	case warningMessage:
		m.warningMsg = msg.msg
		return m, tea.Quit
	case bool:
		if msg {
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *cmdSpinnerUI) View() string {
	spin := m.spinner.View()

	if m.warningMsg != "" {
		spin = "ⓘ"
		m.opts.message = m.warningMsg
		m.opts.message += "\n"
	} else if m.quitting {
		if m.err != nil {
			return m.err.Error()
		}
		spin = "✔"
		m.opts.message += "\n"
	}

	return fmt.Sprintf(m.opts.message, spin, m.opts.instance)
}

type warningMessage struct {
	msg string
}

func initCmdSpinnerUI(fn func() tea.Msg, opts cOpts) *cmdSpinnerUI {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return &cmdSpinnerUI{
		spinner: s,
		cmdFn:   fn,
		opts:    opts,
	}
}

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

// listProfiles - lists profiles with lxc and returns a list of profile names
// attached to the given instance.
func listProfiles(instance string) ([]string, error) {
	var outBuf bytes.Buffer
	cmd := exec.Command("lxc", "config", "show", instance)
	cmd.Stdout = &outBuf

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	type profileInfo struct {
		Profiles []string `yaml:"profiles"`
	}

	var profiles profileInfo
	if err := yaml.Unmarshal(outBuf.Bytes(), &profiles); err != nil {
		return nil, fmt.Errorf("Unable to parse profiles list: %v", err)
	}
	return profiles.Profiles, nil
}

// exportProfile - exports profile from lxc and saves it at dstPath.
func exportProfile(profile, dstPath string) (int64, error) {
	pf, err := os.Create(dstPath)
	if err != nil {
		return -1, fmt.Errorf("Unable to create backup file %s: %v", dstPath, err)
	}
	cmd := exec.Command("lxc", "profile", "show", profile)
	cmd.Stdout = pf
	if err := cmd.Run(); err != nil {
		return -1, err
	}

	// Sync file to disk
	if err := pf.Sync(); err != nil {
		return -1, fmt.Errorf("Error syncing profile file %s to disk: %v", dstPath, err)
	}

	// Save size of the file for showing progress later.
	stat, err := pf.Stat()
	if err != nil {
		return -1, fmt.Errorf("Unable to stat file %s: %v", dstPath, err)
	}

	// Close the file
	if err := pf.Close(); err != nil {
		return -1, fmt.Errorf("Unable to close file %s: %v", dstPath, err)
	}

	return stat.Size(), nil
}

func exportInstance(instance, dstFile string, optimized bool) (int64, error) {
	cmd := exec.Command("lxc", "export", instance, dstFile)
	if optimized {
		cmd = exec.Command("lxc", "export", "--optimized-storage", instance, dstFile)
	}
	cmd.Stdout = ioutil.Discard

	if err := cmd.Run(); err != nil {
		return -1, err
	}

	s, err := os.Stat(dstFile)
	if err != nil {
		return -1, fmt.Errorf("Unable to stat file %s: %v", dstFile, err)
	}
	return s.Size(), nil
}

func fetchExistingProfiles() (s set.StringSet, err error) {
	// First get the list of existing profiles, so we can restore
	// only missing ones.
	var outBuf bytes.Buffer
	cmd := exec.Command("lxc", "profile", "list", "-f", "yaml")
	cmd.Stdout = &outBuf

	if err := cmd.Run(); err != nil {
		return s, err
	}

	type profileInfo struct {
		Name string `yaml:"name"`
	}

	var profileInfos []profileInfo
	if err := yaml.Unmarshal(outBuf.Bytes(), &profileInfos); err != nil {
		return nil, fmt.Errorf("Unable to parse profiles list: %v", err)
	}

	s = set.NewStringSet()
	for _, pi := range profileInfos {
		s.Add(pi.Name)
	}
	return s, nil
}

type warnMsgErr struct {
	msg warningMessage
}

func (w warnMsgErr) Error() string {
	return ""
}

func restoreProfile(ctx *lxminContext, profile, profileKey string, existingProfiles set.StringSet) error {
	proPath := path.Join(ctx.StagingRoot, path.Base(profileKey))

	if existingProfiles.Contains(profile) {
		defer os.Remove(proPath)
		return warnMsgErr{msg: warningMessage{
			msg: `%s Skipping profile ` + profile + ` as it already exists for: %s`,
		}}
	}

	cmd := exec.Command("lxc", "profile", "create", profile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Error creating profile %s: %v", profile, err)
	}

	profileFile, err := os.Open(proPath)
	if err != nil {
		return fmt.Errorf("Error opening backup file %s: %v", proPath, err)
	}

	cmd = exec.Command("lxc", "profile", "edit", profile)
	cmd.Stdin = profileFile
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Error restoring profile %s: %v", profile, err)
	}

	defer os.Remove(proPath)
	return nil
}

func restoreInstance(ctx *lxminContext, bkp backup) (*bytes.Buffer, error) {
	outBuf := bytes.Buffer{}
	localPath := path.Join(ctx.StagingRoot, bkp.backupName+"_instance.tar.gz")

	lastCmd := []string{"lxc", "import", localPath}
	cmd := exec.Command(lastCmd[0], lastCmd[1:]...)
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = &outBuf
	if err := cmd.Run(); err != nil {
		errBuf := bytes.Buffer{}
		errBuf.Write([]byte(
			fmt.Sprintf("Command: %s\n", strings.Join(lastCmd, " ")),
		))
		errBuf.Write(outBuf.Bytes())
		return &errBuf, fmt.Errorf("Error importing instance: %v", err)
	}

	// Clear outBuf for next command
	outBuf = bytes.Buffer{}
	lastCmd = []string{"lxc", "start", bkp.instance}
	cmd = exec.Command(lastCmd[0], lastCmd[1:]...)
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = &outBuf
	if err := cmd.Run(); err != nil {
		errBuf := bytes.Buffer{}
		errBuf.Write([]byte(
			fmt.Sprintf("Command: %s\n", strings.Join(lastCmd, " ")),
		))
		errBuf.Write(outBuf.Bytes())
		return &errBuf, fmt.Errorf("Error starting instance: %v", err)
	}

	defer os.Remove(localPath)
	return nil, nil
}
