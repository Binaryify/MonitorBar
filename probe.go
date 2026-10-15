package main

import (
	"fmt"
	"time"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
)

// probe prints what the app can see and control, without UI (--probe).
func probe() {
	m := display.Target()
	fmt.Printf("显示器: %s  接口: %s\n", m.Name, or(m.Connection, "未知"))
	if mode, ok := m.CurrentMode(); ok {
		hidpi := ""
		if mode.HiDPI() {
			hidpi = " HiDPI"
		}
		fmt.Printf("当前模式: %s%s %dHz\n", mode.Title(), hidpi, mode.RefreshRate)
	}
	fmt.Printf("可用模式: %d 个  亮屏: %v  空闲: %.0f 秒\n", len(m.Modes()), !m.Asleep(), display.IdleSeconds())

	start := time.Now()
	host, err := mitv.Discover()
	if err != nil {
		fmt.Println("局域网: 未发现澎湃 OS 显示器")
		return
	}
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

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
