// Package peer reaches MonitorBar on the other computer wired to the same
// monitors. A monitor takes DDC/CI commands only from the computer it
// shows, so to bring a screen here, this computer asks the other one to
// switch it over and to drop it from its own arrangement.
//
// The transport is ssh with keys (Remote Login on, a key without a
// passphrase): it needs no pairing, no open port and no firewall prompt,
// and both sides already trust it. The other side runs
// "MonitorBar --release" or "--status" and answers one JSON line.
package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DefaultApp is where MonitorBar lives on a Mac.
const DefaultApp = "/Applications/MonitorBar.app/Contents/MacOS/MonitorBar"

// Release is the answer to --release: what the other computer did.
type Release struct {
	Held     bool   `json:"held"`     // the screen was in its arrangement
	Sent     bool   `json:"sent"`     // it sent the input switch over DDC/CI
	Disabled bool   `json:"disabled"` // it dropped the screen from its arrangement
	Error    string `json:"error,omitempty"`
}

// Screen is a display as --status reports it.
type Screen struct {
	Key    string `json:"key"` // "vendor:model"
	Active bool   `json:"active"`
}

// Status is the answer to --status.
type Status struct {
	Host    string   `json:"host"`
	Screens []Screen `json:"screens"`
}

// Has reports whether the screen is in the other computer's arrangement.
func (s Status) Has(key string) bool {
	for _, sc := range s.Screens {
		if sc.Key == key && sc.Active {
			return true
		}
	}
	return false
}

// SSH reaches the other computer with ssh.
type SSH struct {
	Host string // e.g. "mini.local"
	App  string // MonitorBar's path there, DefaultApp when empty
}

// ErrNoHost means no other computer is set.
var ErrNoHost = errors.New("peer: no host set")

func (p SSH) Release(key string, input int) (Release, error) {
	var r Release
	args := []string{"--release", key}
	if input > 0 {
		args = append(args, strconv.Itoa(input))
	}
	err := p.call(20*time.Second, &r, args...)
	return r, err
}

func (p SSH) Status() (Status, error) {
	var s Status
	err := p.call(8*time.Second, &s, "--status")
	return s, err
}

func (p SSH) call(timeout time.Duration, out any, args ...string) error {
	host := strings.TrimSpace(p.Host)
	if host == "" {
		return ErrNoHost
	}
	app := p.App
	if app == "" {
		app = DefaultApp
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	remote := shellQuote(app)
	for _, a := range args {
		remote += " " + shellQuote(a)
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=3", host, remote)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	line := lastLine(stdout.String())
	if err := json.Unmarshal([]byte(line), out); err != nil {
		return errors.New("对方的 MonitorBar 版本不支持协同切换")
	}
	return nil
}

// lastLine skips anything the login shell printed before the answer.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
