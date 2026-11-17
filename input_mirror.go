package main

import (
	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/locales"
	"slices"
	"time"
)

// All controller state belongs to the UI thread. Only DDC transactions run
// in workers, with one outstanding poll/switch per output.
func (s *store) mirrorSuspended() bool {
	if s.output != nil && s.output.switchBusy {
		return true
	}
	if s.output != nil && (s.monitor.Builtin || s.monitor.Key == "") {
		for _, o := range s.outputs {
			if o.mirror != nil && o.mirror.Suspended() {
				return true
			}
		}
	}
	return s.output != nil && (s.monitor.Mirrored() || (s.mirror != nil && s.mirror.Suspended()))
}
func (s *store) pollInputs() {
	if s.mirrorClosing {
		return
	}
	for _, o := range s.outputs {
		pref := s.prefs.display(o.monitor.Key)
		enabled := s.prefs.InputMirroring && pref.InputMirroring
		if !enabled {
			if o.mirror != nil {
				s.on(o, func() {
					err := o.mirror.Stop()
					s.finishMirror(err)
					if err == nil {
						o.mirror = nil
					}
				})
			}
			continue
		}
		if o.mirror == nil {
			o.mirror = o.monitor.InputMirror(pref.macInput(), pref.winInput())
		}
		if o.mirror == nil || o.sourceBusy || o.switchBusy || o.ddc == nil {
			continue
		}
		o.sourceBusy = true
		d := o.ddc
		controller := o.mirror
		go func() {
			code, _, ok := d.Get(display.VCPInput)
			s.post(func() {
				o.sourceBusy = false
				if s.mirrorClosing || o.mirror != controller || o.switchBusy || !slices.Contains(s.outputs, o) {
					return
				}
				p := s.prefs.display(o.monitor.Key)
				if !s.prefs.InputMirroring || !p.InputMirroring {
					return
				}
				s.on(o, func() {
					if !ok {
						s.notice = locales.MirrorReadFailed()
					}
					was := controller.Suspended()
					err := controller.Observe(code, ok, time.Now())
					if controller.Suspended() {
						o.easing = false
					}
					if err != nil || was != controller.Suspended() {
						s.finishMirror(err)
					}
				})
			})
		}()
	}
}
func (s *store) finishMirror(err error) {
	// A failed pre-arm may already have neutralized the external gamma.
	if !s.mirrorSuspended() {
		s.applyGamma()
	}
	if err != nil {
		s.notice = locales.MirrorFailed()
	}
}
func (s *store) stopMirrors() {
	s.mirrorClosing = true
	for _, o := range s.outputs {
		if o.mirror != nil {
			s.on(o, func() { s.finishMirror(o.mirror.Stop()) })
		}
	}
}
