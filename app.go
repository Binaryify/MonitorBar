package main

import (
	"log"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/usage"
)

// chrome is the panel's height besides the scrolling content: arrow,
// header, tabs and footer, with a margin from the screen's edge.
const chrome = 230

// app is the menu bar (notification area) icon and the panel it opens.
type app struct {
	s      *store
	panel  *panel
	tray   *mygo.Tray
	flyout *mygo.Window // the open panel, or nil
}

func (a *app) ready(show bool) {
	a.s = newStore()
	a.s.redraw = func() {
		if a.flyout != nil {
			a.flyout.Invalidate()
		}
	}
	a.panel = &panel{s: a.s, height: 600, maxContent: 640, onHeight: a.resize}

	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(), IconIsTemplate: true, ToolTip: "MonitorBar"})
	if err != nil {
		log.Fatal("tray: ", err)
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "打开 MonitorBar", Click: func(*mygo.MenuItem, *mygo.Window) { a.open() }},
		mygo.Separator(),
		{Label: "退出 MonitorBar", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	if runtime.GOOS == "linux" { // AppIndicator reports no clicks
		a.tray.SetMenu(menu)
	} else {
		a.tray.OnClick(a.toggle)
		a.tray.OnRightClick(func() {
			a.close()
			a.tray.SetMenu(menu) // for this popup only, so clicks keep toggling the panel
			defer a.tray.SetMenu(nil)
			a.tray.PopUpMenu()
		})
	}

	changed := func() {
		a.s.displaysChanged()
		a.s.redraw()
	}
	// Launching the app again (Start menu, Finder) opens its panel.
	mygo.App.OnSecondInstance(func([]string, string) { a.open() })
	mygo.Screen.OnDisplaysChanged(changed)
	mygo.Power.OnResume(changed)
	go func() {
		for range time.Tick(usage.Interval) {
			mygo.RunOnMain(func() {
				a.s.recordUsage()
				a.s.redraw()
			})
		}
	}()
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.s.save()
		display.RestoreGamma()
		if a.s.ddc != nil {
			a.s.ddc.Close()
		}
		a.tray.Destroy()
	})

	a.s.detect()
	a.s.autoCheck()
	// Open the panel on the first launch (or with --show) so new users find the icon.
	if show || !a.s.prefs.LaunchedBefore {
		a.s.prefs.LaunchedBefore, a.s.dirty = true, true
		time.AfterFunc(300*time.Millisecond, func() { mygo.RunOnMain(a.open) })
	}
}

func (a *app) toggle() {
	if a.flyout != nil {
		a.close()
	} else {
		a.open()
	}
}

func (a *app) open() {
	if a.flyout != nil {
		return
	}
	a.s.panelOpened()
	icon := a.tray.Bounds()
	area := mygo.Screen.DisplayNearestPoint(mygo.Point{X: icon.X + icon.Width/2, Y: icon.Y + icon.Height/2}).WorkArea
	a.panel.maxContent = float32(max(area.Height-chrome, 200))
	a.flyout = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Width:     panelWidth,
		Height:    min(a.panel.height, area.Height),
		Focusable: true,
		Shadow:    true,
		Content:   ui.View(a.panel.view),
	})
	a.flyout.OnClosed(func() {
		a.flyout = nil
		a.s.save()
	})
	a.pointArrow()
}

func (a *app) close() {
	if a.flyout != nil {
		a.flyout.Close()
	}
}

// resize fits the panel to its content, which changes with the tab.
func (a *app) resize(height int) {
	if a.flyout == nil {
		return
	}
	if corner, ok := a.corner(height); ok {
		// SetSize would place the flyout again against an icon whose place
		// is unknown, in the middle of the screen, before pointArrow moved
		// it: set the size and the place at once, so it doesn't flash.
		a.flyout.SetBounds(corner)
		return
	}
	a.flyout.SetSize(panelWidth, height)
	a.pointArrow()
}

// pointArrow aims the arrow at the icon's middle, wherever the panel was
// placed against the screen's edges.
func (a *app) pointArrow() {
	if corner, ok := a.corner(a.flyout.Bounds().Height); ok {
		a.flyout.SetBounds(corner)
		return
	}
	icon, win := a.tray.Bounds(), a.flyout.Bounds()
	a.panel.arrowX = float32(icon.X + icon.Width/2 - win.X)
	a.flyout.Invalidate()
}

// corner is where the panel goes when the icon's place is unknown (Windows
// keeps it in the notification area's overflow): the bottom right corner,
// where Windows puts the panels of its own icons.
func (a *app) corner(height int) (mygo.Rectangle, bool) {
	if runtime.GOOS != "windows" || a.tray.Bounds().Width != 0 {
		return mygo.Rectangle{}, false
	}
	area := mygo.Screen.PrimaryDisplay().WorkArea
	height = min(height, area.Height-24)
	return mygo.Rectangle{X: area.X + area.Width - panelWidth - 12, Y: area.Y + area.Height - height - 12,
		Width: panelWidth, Height: height}, true
}
