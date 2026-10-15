// MonitorBar manages an external monitor from the macOS menu bar or the
// Windows notification area: inputs and volume over the HyperOS LAN API,
// software brightness and warmth, resolutions and screen time.
//
//	MonitorBar           run, panel opens on first launch
//	MonitorBar --show    run and open the panel
//	MonitorBar --probe   print what it can see and control, then exit
//	MonitorBar --open-at-login on|off   start at login or not, then exit
//	MonitorBar --pull    bring the shared screens to this computer
//	MonitorBar --release <vendor:model> [input]   (run by the other computer)
//	MonitorBar --status  list this computer's displays as JSON
package main

import (
	"fmt"
	"log"
	"os"
	"slices"

	"github.com/egoist/mygo"
)

func main() {
	if slices.Contains(os.Args[1:], "--probe") {
		probe()
		return
	}
	// Run by the other computer over ssh: answer and exit, beside a
	// running app.
	if i := slices.Index(os.Args, "--release"); i > 0 {
		releaseCommand(os.Args[i+1:])
		return
	}
	if slices.Contains(os.Args[1:], "--status") {
		statusCommand()
		return
	}
	mygo.App.SetName("MonitorBar")
	if i := slices.Index(os.Args, "--open-at-login"); i > 0 && i+1 < len(os.Args) {
		setOpenAtLogin(os.Args[i+1] == "on")
		return
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.OnWindowAllClosed(func() {}) // the panel is the only window: keep running as it closes
	a := &app{}
	show := slices.Contains(os.Args[1:], "--show")
	pullNow := slices.Contains(os.Args[1:], "--pull")
	mygo.App.WhenReady(func() { a.ready(show, pullNow) })
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
