package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/binaryify/MonitorBar/internal/mitv"
	"github.com/binaryify/MonitorBar/locales"
)

func TestOverviewSyncGrouping(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	peer := &store{prefs: s.prefs, parent: s, link: mitv.New("192.0.2.2"), hasVolume: true, volume: 35}
	s.peers = []*store{peer}
	s.prefs.DeviceNames = map[string]string{s.link.Host: "左护法", peer.link.Host: "右护法"}
	s.prefs.SyncVolume = true
	s.prefs.SyncPicture = true
	groups := s.overviewVolumes()
	if len(groups) != 1 || len(groups[0]) != 2 {
		t.Fatal("synced devices should share one control")
	}
	tt := newTester(p)
	if !tt.HasText("左护法 + 右护法") || !tt.HasText(locales.SyncedVolume()) || !tt.HasText(locales.SyncedBrightness()) {
		t.Fatal(tt.Texts())
	}
	if tt.HasText(locales.Volume()) {
		t.Fatal("duplicate independent volume control")
	}
	s.prefs.DeviceNames[peer.link.Host] = "右屏"
	tt.Frame()
	if !tt.HasText("左护法 + 右屏") {
		t.Fatal("renamed device not reflected")
	}
	s.prefs.SyncVolume = false
	if len(s.overviewVolumes()) != 2 {
		t.Fatal("independent devices collapsed")
	}
	s.prefs.SyncVolume = true
	s.prefs.ExcludedAudio = map[string]bool{peer.link.Host: true}
	if len(s.overviewVolumes()) != 2 {
		t.Fatal("excluded device collapsed")
	}
	delete(s.prefs.ExcludedAudio, peer.link.Host)
	s.offline = true
	if s.overviewVolumes()[0][0] != peer {
		t.Fatal("offline leader blocks online device")
	}
	s.hasVolume = false
	if len(s.overviewVolumes()) != 1 {
		t.Fatal("peer hidden by main device without volume")
	}
}

func TestBoundScreenAlias(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.prefs.ScreenBindings = map[string]string{s.link.Host: s.monitor.Key()}
	s.prefs.DeviceNames = map[string]string{s.link.Host: "左护法"}
	if got := s.screenLabel(0, s.monitor.Key(), s.monitor.Name); got != "左护法" {
		t.Fatal(got)
	}
	if p.headerTitle() != "左护法" {
		t.Fatal("header ignores alias")
	}
	s.prefs.DeviceNames[s.link.Host] = "左屏"
	if p.headerTitle() != "左屏" {
		t.Fatal("header kept stale alias")
	}
	if got := s.screenName("unbound", "Mi Monitor"); got != "Mi Monitor" {
		t.Fatal("guessed unbound identity")
	}
}

func TestOverviewGroupSnapshots(t *testing.T) {
	dir := os.Getenv("MONITORBAR_SNAPSHOTS")
	if dir == "" {
		t.Skip("set MONITORBAR_SNAPSHOTS")
	}
	old := testLocale
	defer func() { testLocale = old }()
	for _, lang := range []string{"en", "zh-Hans"} {
		testLocale = lang
		p, _ := testPanel(t)
		s := p.s
		s.peers = []*store{{prefs: s.prefs, parent: s, link: mitv.New("192.0.2.2"), hasVolume: true, volume: 80}}
		s.prefs.DeviceNames = map[string]string{s.link.Host: "左护法", "192.0.2.2": "右护法"}
		s.prefs.SyncVolume = true
		s.prefs.SyncPicture = true
		tt := newTester(p)
		tt.SetScale(2)
		tt.Frame()
		img := tt.Image()
		f, err := os.Create(filepath.Join(dir, "group-"+lang+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, img.SubImage(image.Rect(0, 0, img.Bounds().Dx(), p.height*2)))
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
