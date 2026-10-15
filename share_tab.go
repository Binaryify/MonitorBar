package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
	"github.com/binaryify/MonitorBar/internal/peer"
)

// MARK: 共享

func (p *panel) sharing(c *ui.Context) {
	p.pullCard(c, true)
	p.peerCard(c)
	p.sharedSettings(c)
}

// pullCard brings the shared screens here; detailed, it lists each one with
// its own button and where it stands.
func (p *panel) pullCard(c *ui.Context, detailed bool) {
	s := p.s
	trailing := ""
	if s.share.shortcutOn != "" {
		trailing = "快捷键 " + shortcutLabel(s.share.shortcutOn)
	}
	card(c.Key("pull"), func() {
		cardTitle(c, "切到本机", "swap", trailing)
		if len(s.prefs.Shared) == 0 {
			ui.Text(c, "还没有共享屏。在「共享屏设置」里打开两台电脑共用的显示器。").FontSize(10).TextColor(muted)
			return
		}
		if detailed {
			ui.Column(c).Gap(8).Children(func() {
				for _, sc := range s.prefs.Shared {
					p.sharedRow(c.Key(sc.Key), sc)
				}
			})
		}
		btn := ui.Box(c.Key("pull-all")).Height(36).Radius(10).Center().Cursor(ui.CursorPointer).Label("全部切到本机")
		if s.share.pulling {
			btn.Background(ui.RGBA(255, 255, 255, 0.08))
		} else {
			btn.Gradient(accent, accentDeep, 180)
		}
		btn.Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if s.share.pulling {
					ui.Spinner(c).Size(12, 12)
					ui.Text(c, "正在切换…").FontSize(12).FontWeight(600)
					return
				}
				icon(c, "swap").FontSize(13).TextColor(ui.RGB(255, 255, 255))
				ui.Text(c, "全部切到本机").FontSize(12).FontWeight(600).TextColor(ui.RGB(255, 255, 255))
			})
		})
		if btn.Clicked() {
			s.pullAll("按钮")
		}
		for _, line := range s.share.results {
			ui.Text(c, line).FontSize(10).TextColor(muted)
		}
	})
}

func (p *panel) sharedRow(c *ui.Context, sc sharedScreen) {
	s := p.s
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(3).Grow(1).Children(func() {
			ui.Text(c, sc.Name).FontSize(12).FontWeight(600).SingleLine()
			channel := "DDC/CI"
			if sc.Channel == channelLAN {
				channel = "局域网"
			}
			states := []string{channel + " · 本机 " + inputName(sc, s.inputs), s.localState(sc.Key)}
			if peer := s.peerState(sc.Key); peer != "" {
				states = append(states, peer)
			}
			ui.Text(c, strings.Join(states, " · ")).FontSize(10).TextColor(muted).SingleLine()
		})
		if chipText(c.Key("go"), "切到本机", false) && !s.share.pulling {
			s.pullScreen(sc.Key)
		}
	})
}

// peerCard sets and checks the other computer.
func (p *panel) peerCard(c *ui.Context) {
	s := p.s
	status := "未设置"
	switch {
	case s.prefs.PeerHost == "":
	case s.share.checking:
		status = "检测中…"
	case s.share.peer != nil:
		status = "在线"
	case s.share.peerErr != "":
		status = "无法连接"
	}
	card(c.Key("peer"), func() {
		cardTitle(c, "另一台电脑", "laptop", status)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c.Key("peer-host"), &s.prefs.PeerHost).Placeholder("主机名，如 mini.local").Label("另一台电脑的主机名").Grow(1)
			if in.Changed() {
				s.dirty = true
			}
			if in.Submitted() {
				s.checkPeer()
			}
			if chipText(c.Key("peer-check"), "检测", false) {
				s.checkPeer()
			}
		})
		switch {
		case s.share.peer != nil:
			ui.Text(c, fmt.Sprintf("%s · %d 块外接屏在排列中", s.share.peer.Host, activeCount(s.share.peer.Screens))).FontSize(10).TextColor(muted)
		case s.share.peerErr != "":
			ui.Text(c, s.share.peerErr).FontSize(10).TextColor(muted)
		}
		ui.Text(c, "两台电脑都要打开「远程登录」，能用 ssh 密钥互相免密登录，并装有这个版本的 MonitorBar。").FontSize(10).TextColor(muted)
	})
}

// sharedSettings picks the shared screens, how each switches and which
// input is this computer's, and the shortcut.
func (p *panel) sharedSettings(c *ui.Context) {
	s := p.s
	card(c.Key("shared-settings"), func() {
		cardTitle(c, "共享屏设置", "sliders", "")
		choices := s.sharedChoices()
		if len(choices) == 0 {
			ui.Text(c, "没有连接外接显示器").FontSize(10).TextColor(muted)
		}
		for _, sc := range choices {
			p.sharedSetting(c.Key(sc.Key()), sc)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			icon(c, "keyboard").FontSize(13).TextColor(muted)
			ui.Text(c, "全部切到本机的快捷键").FontSize(11).Grow(1)
			choice := s.prefs.PullShortcut
			if choice == "" {
				choice = "关闭"
			}
			labels := make([]string, len(shortcutChoices))
			for i, acc := range shortcutChoices {
				labels[i] = shortcutLabel(acc)
			}
			label := shortcutLabel(choice)
			if ui.Select(c.Key("shortcut"), &label, labels).Label("快捷键").Width(110).Changed() {
				for i, l := range labels {
					if l == label {
						s.prefs.PullShortcut, s.dirty = shortcutChoices[i], true
					}
				}
				s.registerShortcut()
			}
		})
	})
}

func (p *panel) sharedSetting(c *ui.Context, sc display.Screen) {
	s := p.s
	i := s.sharedIndex(sc.Key())
	ui.Column(c).Gap(8).Padding(8).Radius(9).Background(ui.RGBA(255, 255, 255, 0.04)).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Text(c, screenLabel(s.prefs.Shared, sc)).FontSize(12).FontWeight(600).SingleLine()
				detail := "型号 " + sc.Key()
				if sc.ID == 0 {
					detail += " · 未连接"
				}
				ui.Text(c, detail).FontSize(10).TextColor(muted)
			})
			on := i >= 0
			if ui.Switch(c.Key("on"), &on).Label("共享这块屏").Changed() {
				s.setShared(sc, on)
				s.registerShortcut()
			}
		})
		if i < 0 {
			return
		}
		entry := &s.prefs.Shared[i]
		row := func(label string, body func()) {
			ui.Row(c.Key(label)).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).FontSize(11).Width(64)
				ui.Spacer(c)
				body()
			})
		}
		row("名称", func() {
			if ui.TextInput(c.Key("name"), &entry.Name).Label("名称").Width(150).Changed() {
				s.dirty = true
			}
		})
		row("切换方式", func() {
			channels := []string{"DDC/CI"}
			if s.link != nil || entry.Channel == channelLAN {
				channels = append([]string{"局域网"}, channels...)
			}
			current := map[string]string{channelLAN: "局域网", channelDDC: "DDC/CI"}[entry.Channel]
			if ui.Select(c.Key("channel"), &current, channels).Label("切换方式").Width(150).Changed() {
				entry.Channel = map[string]string{"局域网": channelLAN, "DDC/CI": channelDDC}[current]
				if list := p.inputChoices(*entry); len(list) > 0 {
					entry.Input = list[0].ID
				}
				s.dirty = true
			}
		})
		row("本机接口", func() {
			list := p.inputChoices(*entry)
			names := make([]string, len(list))
			for j, src := range list {
				names[j] = src.Name
			}
			current := inputName(*entry, s.inputs)
			if ui.Select(c.Key("input"), &current, names).Label("本机接口").Width(150).Changed() {
				for _, src := range list {
					if src.Name == current {
						entry.Input, s.dirty = src.ID, true
					}
				}
			}
		})
	})
}

// inputChoices are the inputs a shared screen can name as this computer's.
func (p *panel) inputChoices(sc sharedScreen) []mitv.Source {
	if sc.Channel == channelLAN {
		return p.s.inputs
	}
	return ddcInputs
}

func activeCount(screens []peer.Screen) int {
	n := 0
	for _, sc := range screens {
		if sc.Active {
			n++
		}
	}
	return n
}

// shortcutLabel shows an accelerator the Mac way: "⌃⌥⌘M".
func shortcutLabel(acc string) string {
	if acc == "关闭" || acc == "" {
		return "关闭"
	}
	r := strings.NewReplacer("Ctrl+", "⌃", "Alt+", "⌥", "Shift+", "⇧", "Cmd+", "⌘", "CmdOrCtrl+", "⌘")
	return r.Replace(acc)
}
