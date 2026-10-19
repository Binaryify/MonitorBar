package main

import (
	"log"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/usage"
	"github.com/binaryify/MonitorBar/locales"
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
	closed time.Time    // when the panel last closed

	hud      *mygo.Window // the level a key set, or nil
	hudKind  hudKind
	hudUntil time.Time // when the HUD goes
}

func (a *app) ready(show bool) {
	a.s = newStore()
	a.s.redraw = func() {
		if a.flyout != nil {
			a.flyout.Invalidate()
		}
	}
	a.panel = &panel{s: a.s, height: 600, maxContent: 640, onHeight: a.resize}
	a.s.onAdjust = a.showHUD

	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(), IconIsTemplate: true, ToolTip: "MonitorBar"})
	if err != nil {
		log.Fatal("tray: ", err)
	}
	// The menu is made anew in the language of the moment.
	menu := func() *mygo.Menu {
		return mygo.NewMenu([]*mygo.MenuItem{
			{Label: locales.TrayOpen(), Click: func(*mygo.MenuItem, *mygo.Window) { a.open() }},
			mygo.Separator(),
			{Label: locales.MenuQuit(), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		})
	}
	if runtime.GOOS == "linux" { // AppIndicator reports no clicks
		a.tray.SetMenu(menu())
		mygo.App.OnLocaleChanged(func(string) { a.tray.SetMenu(menu()) })
	} else {
		a.tray.OnClick(a.toggle)
		a.tray.OnRightClick(func() {
			a.close()
			a.tray.SetMenu(menu()) // for this popup only, so clicks keep toggling the panel
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

	a.s.bindShortcuts = true
	a.s.registerShortcuts()
	a.s.syncMediaKeys()
	a.s.detect()
	a.s.autoCheck()
	go func() { // the built-in display's brightness, for following it, and the media keys' permission
		for range time.Tick(time.Second) {
			mygo.RunOnMain(func() {
				denied := a.s.mediaKeysDenied
				if a.s.syncMediaKeys(); denied != a.s.mediaKeysDenied {
					a.s.redraw()
				}
				if a.s.prefs.FollowBuiltin && a.s.canFollow() {
					a.s.followTick(display.BuiltinBrightness)
					a.s.redraw()
				}
			})
		}
	}()
	go func() { // a newer version: soon after launch, then every few hours
		time.Sleep(10 * time.Second)
		for {
			mygo.RunOnMain(func() { a.s.checkUpdate(false) })
			time.Sleep(6 * time.Hour)
		}
	}()
	// Open the panel on the first launch (or with --show) so new users find the icon.
	if show || !a.s.prefs.LaunchedBefore {
		a.s.prefs.LaunchedBefore, a.s.dirty = true, true
		time.AfterFunc(300*time.Millisecond, func() { mygo.RunOnMain(a.open) })
	}
}

// toggle opens the panel, or closes it. Recent macOS closes the panel as
// the icon is pressed, a press outside it, before the click gets here: that
// click only closes it, as AppKit's popovers do.
func (a *app) toggle() {
	switch {
	case a.flyout != nil:
		a.close()
	case time.Since(a.closed) > 500*time.Millisecond:
		a.open()
	}
}

func (a *app) open() {
	if a.flyout != nil {
		return
	}
	if a.hud != nil {
		a.hud.Close()
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
		a.flyout, a.closed = nil, time.Now()
		a.s.record("") // a shortcut left recording keeps its keys
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
	if corner, ok := a.corner(panelWidth, height); ok {
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
	if corner, ok := a.corner(panelWidth, a.flyout.Bounds().Height); ok {
		a.flyout.SetBounds(corner)
		return
	}
	icon, win := a.tray.Bounds(), a.flyout.Bounds()
	a.panel.arrowX = float32(icon.X + icon.Width/2 - win.X)
	a.flyout.Invalidate()
}

// corner is where the panel (or the HUD) goes when the icon's place is
// unknown (Windows keeps it in the notification area's overflow): the
// bottom right corner, where Windows puts the panels of its own icons.
func (a *app) corner(width, height int) (mygo.Rectangle, bool) {
	if runtime.GOOS != "windows" || a.tray.Bounds().Width != 0 {
		return mygo.Rectangle{}, false
	}
	area := mygo.Screen.PrimaryDisplay().WorkArea
	height = min(height, area.Height-24)
	return mygo.Rectangle{X: area.X + area.Width - width - 12, Y: area.Y + area.Height - height - 12,
		Width: width, Height: height}, true
}
