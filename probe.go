package main

import (
	"fmt"
	"time"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
)

// probe prints what the app can see and control, without UI (--probe).
func probe() {
	for i, m := range display.Monitors() {
		if i > 0 {
			fmt.Println()
		}
		kind := "外接"
		if m.Builtin {
			kind = "内置"
		}
		fmt.Printf("显示器: %s  接口: %s  类型: %s\n", m.Name, or(m.Connection, "未知"), kind)
		if mode, ok := m.CurrentMode(); ok {
			hidpi := ""
			if mode.HiDPI() {
				hidpi = " HiDPI"
			}
			fmt.Printf("当前模式: %s%s %dHz\n", mode.Title(), hidpi, mode.RefreshRate)
		}
		modes := m.Modes()
		current, _ := m.CurrentMode()
		w, h := m.NativeSize(modes, current)
		fmt.Printf("原生分辨率: %d × %d（%s）\n", w, h, display.SizeLabel(w, h))
		fmt.Printf("可用模式: %d 个  亮屏: %v  空闲: %.0f 秒\n", len(modes), !m.Asleep(), display.IdleSeconds())

		if d, err := m.OpenDDC(); err != nil {
			fmt.Println("DDC/CI: 显示器未响应")
		} else {
			for _, v := range []struct {
				code byte
				name string
			}{{display.VCPBrightness, "亮度"}, {display.VCPContrast, "对比度"}, {display.VCPVolume, "音量"}, {display.VCPInput, "输入源"}} {
				if cur, max, ok := d.Get(v.code); ok {
					fmt.Printf("DDC/CI %s: %d / %d\n", v.name, cur, max)
				}
			}
			caps := d.Capabilities()
			fmt.Println("DDC/CI 能力:", or(caps, "（未提供）"))
			if ids := display.ParseCapabilities(caps)[display.VCPInput]; len(ids) > 0 {
				fmt.Print("DDC/CI 输入源列表:")
				for _, id := range ids {
					fmt.Printf(" %s=0x%02X", display.InputNameFor(m.Name, id), id)
				}
				fmt.Println()
			}
			d.Close()
		}
	}

	start := time.Now()
	hosts := mitv.DiscoverAll()
	if len(hosts) == 0 {
		fmt.Println("局域网: 未发现澎湃 OS 显示器")
		return
	}
	for _, host := range hosts {
		link := mitv.New(host)
		name, _ := link.DeviceName()
		fmt.Printf("局域网: %s @ %s（%.2fs）\n", name, host, time.Since(start).Seconds())
		if sources, err := link.Sources(); err == nil {
			fmt.Print("信号源:")
			for _, s := range sources {
				fmt.Printf(" %s=%d", s.Name, s.ID)
			}
			fmt.Println()
		}
		if v, err := link.Volume(); err == nil {
			fmt.Printf("音量: %d\n", v)
		}
	}
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
