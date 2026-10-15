package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
	"github.com/binaryify/MonitorBar/internal/peer"
)

// Sharing: monitors wired to this computer and to another one. "切到本机"
// brings them here: the LAN remote switches a HyperOS monitor from either
// side; a DDC/CI monitor takes the switch only from the computer it shows,
// so the other one sends it. Each side also drops the screens it gave
// away from its arrangement, so the pointer can't wander onto a screen
// showing the other computer.

// sharedScreen is a monitor this computer shares, as set in the 共享 tab.
type sharedScreen struct {
	Name    string `json:"name"`
	Key     string `json:"key"`     // EDID "vendor:model"
	Channel string `json:"channel"` // channelLAN or channelDDC
	Input   int    `json:"input"`   // this computer's input: a LAN source ID or an MCCS code
}

const (
	channelLAN = "lan"
	channelDDC = "ddc"
)

// defaultShortcut brings every shared screen here.
const defaultShortcut = "Ctrl+Alt+Cmd+M"

var shortcutChoices = []string{"Ctrl+Alt+Cmd+M", "Ctrl+Alt+Cmd+S", "Ctrl+Alt+Cmd+1", "关闭"}

// ddcInputs are the MCCS input codes offered for a DDC/CI screen.
var ddcInputs = []mitv.Source{
	{ID: 0x0F, Name: "DP 1"}, {ID: 0x10, Name: "DP 2 / USB-C"}, {ID: 0x11, Name: "HDMI 1"},
	{ID: 0x12, Name: "HDMI 2"}, {ID: 0x1B, Name: "USB-C"},
}

// MARK: Screens given away

// The IDs of the screens this computer turned off, kept in a file both the
// app and the --release command use: a disabled display may drop out of
// the system's list, and the ID is how it comes back.

func disabledPath() string { return filepath.Join(dataDir(), "disabled.json") }

func loadDisabled() map[string]uint32 {
	m := map[string]uint32{}
	if data, err := os.ReadFile(disabledPath()); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

func setDisabled(key string, id uint32) {
	m := loadDisabled()
	if id == 0 {
		delete(m, key)
	} else {
		m[key] = id
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	_ = os.MkdirAll(dataDir(), 0o755)
	_ = os.WriteFile(disabledPath(), data, 0o644)
}

// shareLog appends a line to share.log, for finding out afterwards what a
// shortcut press or a request from the other computer did.
func shareLog(format string, args ...any) {
	_ = os.MkdirAll(dataDir(), 0o755)
	f, err := os.OpenFile(filepath.Join(dataDir(), "share.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// MARK: Commands the other computer runs

// releaseCommand is "--release <key> [input]": send the input switch over
// DDC/CI when given (this computer shows the screen, so the monitor
// listens), then drop the screen from the arrangement. Prints one line of
// JSON.
func releaseCommand(args []string) {
	var r peer.Release
	defer func() {
		out, _ := json.Marshal(r)
		fmt.Println(string(out))
		shareLog("release %v -> %s", args, out)
	}()
	if len(args) == 0 {
		r.Error = "缺少显示器"
		return
	}
	key, input := args[0], 0
	if len(args) > 1 {
		input, _ = strconv.Atoi(args[1])
	}
	screens := display.OnlineScreens()
	sc, ok := display.FindScreen(screens, key)
	if !ok || !sc.Active {
		return // not here: nothing to send, nothing to drop
	}
	r.Held = true
	if input > 0 {
		d, err := display.OpenScreenDDC(sc.ID)
		if err != nil {
			r.Error = "找不到这台显示器的 DDC/CI 通道"
			return
		}
		r.Sent = d.SetNow(display.VCPInput, input)
		d.Close()
		if !r.Sent {
			r.Error = "显示器没有接受切换命令"
			return
		}
		time.Sleep(2 * time.Second) // let the monitor change over before the picture stops
	}
	if !othersActive(screens, sc.ID) {
		r.Error = "这是唯一亮着的屏幕，保留在排列里"
		return
	}
	if err := display.SetScreenEnabled(sc.ID, false); err != nil {
		r.Error = "系统拒绝移出这块屏幕"
		return
	}
	setDisabled(key, sc.ID)
	r.Disabled = true
}

// statusCommand is "--status": the displays in this computer's arrangement.
func statusCommand() {
	host, _ := os.Hostname()
	st := peer.Status{Host: host}
	for _, sc := range display.OnlineScreens() {
		if !sc.Builtin {
			st.Screens = append(st.Screens, peer.Screen{Key: sc.Key(), Active: sc.Active})
		}
	}
	out, _ := json.Marshal(st)
	fmt.Println(string(out))
}

func othersActive(screens []display.Screen, id uint32) bool {
	for _, s := range screens {
		if s.ID != id && s.Active {
			return true
		}
	}
	return false
}

// MARK: Bringing screens here

// shareOps are the system calls pull makes, replaced in tests.
type shareOps struct {
	screens   func() []display.Screen
	enable    func(id uint32) error
	disabled  func() map[string]uint32
	forget    func(key string)
	selectLAN func(source int) error
	release   func(key string, input int) (peer.Release, error)
	sleep     func(time.Duration)
}

// linkWait is how long a screen turned back on may take to light up.
const linkWait = 8 * time.Second

// pull brings the screens here, one line of outcome each. It blocks for
// seconds: run it off the main thread.
func pull(ops shareOps, screens []sharedScreen) []string {
	var out []string
	for _, sc := range screens {
		out = append(out, sc.Name+"："+pullOne(ops, sc))
	}
	return out
}

func pullOne(ops shareOps, sc sharedScreen) string {
	if !ensureLocal(ops, sc.Key) {
		return "本机没有这块屏的信号链路。拔掉本机其他外接屏的线，等它亮起后再插回。"
	}
	switch sc.Channel {
	case channelLAN:
		if err := ops.selectLAN(sc.Input); err != nil {
			return "局域网切换失败，显示器没有响应"
		}
		// The monitor listens to anyone here; the other side only has to
		// let the screen go from its arrangement.
		if _, err := ops.release(sc.Key, 0); err != nil {
			return "已切到本机（对方不在线，它的排列里还留着这块屏）"
		}
		return "已切到本机"
	default:
		r, err := ops.release(sc.Key, sc.Input)
		switch {
		case err != nil:
			return "对方不在线，无法切换：" + err.Error()
		case !r.Held:
			return "不在对方那边，应该已经在本机"
		case r.Error != "":
			return r.Error
		}
		return "已切到本机"
	}
}

// ensureLocal makes sure the screen is in this computer's arrangement:
// turned back on if this computer gave it away, then given time to light.
func ensureLocal(ops shareOps, key string) bool {
	if sc, ok := display.FindScreen(ops.screens(), key); ok && sc.Active {
		return true
	}
	if id := ops.disabled()[key]; id != 0 {
		_ = ops.enable(id)
	}
	for waited := time.Duration(0); waited < linkWait; waited += 500 * time.Millisecond {
		if sc, ok := display.FindScreen(ops.screens(), key); ok && sc.Active {
			ops.forget(key)
			return true
		}
		ops.sleep(500 * time.Millisecond)
	}
	return false
}

// systemOps are the real calls.
func systemOps(link *mitv.Link, p peer.SSH) shareOps {
	return shareOps{
		screens:  display.OnlineScreens,
		enable:   func(id uint32) error { return display.SetScreenEnabled(id, true) },
		disabled: loadDisabled,
		forget:   func(key string) { setDisabled(key, 0) },
		selectLAN: func(source int) error {
			if link == nil {
				return fmt.Errorf("no LAN remote")
			}
			return link.Select(source)
		},
		release: p.Release,
		sleep:   time.Sleep,
	}
}

// screenLabel names a screen for the settings: the configured name, else
// its model number.
func screenLabel(screens []sharedScreen, sc display.Screen) string {
	for _, s := range screens {
		if s.Key == sc.Key() {
			return s.Name
		}
	}
	return "显示器 " + strconv.Itoa(int(sc.Model))
}

// inputName names this computer's input of a shared screen.
func inputName(sc sharedScreen, lan []mitv.Source) string {
	list := ddcInputs
	if sc.Channel == channelLAN {
		list = lan
	}
	for _, s := range list {
		if s.ID == sc.Input {
			return s.Name
		}
	}
	return strings.TrimSpace(fmt.Sprintf("输入 %d", sc.Input))
}
