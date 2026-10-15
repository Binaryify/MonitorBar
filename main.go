// MonitorBar manages connected monitors from the macOS menu bar or the
// Windows notification area: inputs and volume over the HyperOS LAN API,
// software brightness and warmth, resolutions and screen time. Each display
// keeps its own picture and control channel.
//
//	MonitorBar           run, panel opens on first launch
//	MonitorBar --show    run and open the panel
//	MonitorBar --probe   print what it can see and control, then exit
//	MonitorBar --open-at-login on|off   start at login or not, then exit
//	MonitorBar --update  install a newer version if there is one, then exit
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"time"

	"github.com/egoist/mygo"
)

func main() {
	if slices.Contains(os.Args[1:], "--probe") {
		probe()
		return
	}
	mygo.App.SetName("MonitorBar")
	if i := slices.Index(os.Args, "--open-at-login"); i > 0 && i+1 < len(os.Args) {
		setOpenAtLogin(os.Args[i+1] == "on")
		return
	}
	if slices.Contains(os.Args[1:], "--update") {
		update()
		return
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.OnWindowAllClosed(func() {}) // the panel is the only window: keep running as it closes
	a := &app{}
	show := slices.Contains(os.Args[1:], "--show")
	mygo.App.WhenReady(func() { a.ready(show) })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// setOpenAtLogin is a command: the login item needs the app's event loop,
// so it runs one, changes the setting, reports it and quits.
func setOpenAtLogin(on bool) {
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.WhenReady(func() {
		if err := mygo.App.SetOpenAtLogin(on); err != nil {
			fmt.Println("开机自启设置失败:", err)
		}
		fmt.Println("开机自启:", map[bool]string{true: "已开启", false: "已关闭"}[mygo.App.OpenAtLogin()])
		mygo.App.Quit()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// update is a command: it installs a newer version, which runs from the
// next launch.
func update() {
	if !mygo.Updater.Enabled() {
		fmt.Println("这个版本不能自动更新（开发版本，或安装目录不可写）")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	up, err := mygo.Updater.Check(ctx)
	switch {
	case err != nil:
		fmt.Println("检查更新失败:", err)
		os.Exit(1)
	case up == nil:
		fmt.Println("已是最新版本", mygo.App.Version())
		return
	}
	fmt.Printf("正在从 %s 更新到 %s…\n", mygo.App.Version(), up.Version)
	shown := -1
	err = up.Install(ctx, func(done, total int64) {
		if p := int(done * 100 / max(total, 1)); p/10 != shown/10 {
			shown = p
			fmt.Printf("  %d%%\n", p)
		}
	})
	if err != nil {
		fmt.Println("更新失败:", err)
		os.Exit(1)
	}
	fmt.Println("已更新到", up.Version, "，重新打开 MonitorBar 生效")
}
