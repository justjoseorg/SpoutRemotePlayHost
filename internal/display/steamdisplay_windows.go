//go:build windows

package display

import (
	"log"
	"strings"
	"sync"
	"time"
)

// Steam creates its own SudoVDA virtual display for Remote Play. This backend does not create
// one: it waits for that display to appear, turns the other displays off, and puts the saved
// layout back when the session ends.
const (
	virtualWaitTimeout = 20 * time.Second
	virtualWaitPeriod  = 500 * time.Millisecond

	// watchPeriod is how often the session layout is checked, reisolateBackoff the least time
	// between two re-isolations so we never fight Windows in a tight loop.
	watchPeriod      = 500 * time.Millisecond
	reisolateBackoff = 2 * time.Second
)

// topo serialises layout changes made by the session goroutine and destroy.
var topo sync.Mutex

type targetRef struct {
	Adapter luid
	ID      uint32
}

// The layout from before the current session, and the session's virtual display.
var layout struct {
	sync.Mutex
	paths []pathInfo
	modes []modeInfo
	virt  *targetRef
}

func setSessionLayout(paths []pathInfo, modes []modeInfo, virt *targetRef) {
	layout.Lock()
	defer layout.Unlock()
	layout.paths, layout.modes, layout.virt = paths, modes, virt
}

func sessionLayout() ([]pathInfo, []modeInfo, *targetRef) {
	layout.Lock()
	defer layout.Unlock()
	return append([]pathInfo(nil), layout.paths...), append([]modeInfo(nil), layout.modes...), layout.virt
}

type steamDisplay struct {
	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

func newBackend() backend { return &steamDisplay{} }

// virtualName reports whether a monitor name or device path belongs to the SudoVDA driver.
// Steam names its display after the client (e.g. "ayn-odin-2-po"), so the reliable marker is
// SudoVDA's EDID manufacturer ID, SMK, in the device path (\\?\DISPLAY#SMKD1CE#...).
func virtualName(s string) bool {
	s = strings.ToLower(s)
	for _, k := range []string{"display#smk", "sudovda", "sudomaker", "virtual display"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// findVirtual returns the target of a SudoVDA display, if Windows lists one.
func findVirtual() (*targetRef, error) {
	paths, _, err := queryConfig(qdcAllPaths)
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		if p.Target.TargetAvailable == 0 {
			continue
		}
		friendly, path := targetName(p.Target.Adapter, p.Target.ID)
		if virtualName(friendly) || virtualName(path) {
			return &targetRef{p.Target.Adapter, p.Target.ID}, nil
		}
	}
	return nil, nil
}

// create returns at once; Steam may take a while to add its display, and the session request
// must not block on that. The requested Mode is ignored because Steam sizes its own display.
func (s *steamDisplay) create(Mode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stop, s.done = make(chan struct{}), make(chan struct{})
	go s.run(s.stop, s.done)
	return nil
}

func (s *steamDisplay) run(stop, done chan struct{}) {
	defer close(done)
	deadline := time.Now().Add(virtualWaitTimeout)
	var virt *targetRef
	for virt == nil {
		var err error
		if virt, err = findVirtual(); err != nil {
			log.Println("could not list displays:", err)
		}
		if virt != nil {
			break
		}
		if time.Now().After(deadline) {
			log.Println("Steam's virtual display did not appear; leaving the physical displays on")
			return
		}
		select {
		case <-stop:
			return
		case <-time.After(virtualWaitPeriod):
		}
	}

	topo.Lock()
	select {
	case <-stop:
		topo.Unlock()
		return
	default:
	}
	origPaths, origModes, err := queryConfig(qdcOnlyActivePaths)
	if err != nil {
		log.Println("could not save the display layout:", err)
	}
	setSessionLayout(origPaths, origModes, virt)
	if err := isolate(virt.Adapter, virt.ID, keepOutputs()); err != nil {
		log.Println("other displays not turned off:", err)
	}
	topo.Unlock()
	watchLayout(virt.Adapter, virt.ID, stop)
}

// watchLayout re-isolates the virtual display while the session runs if Windows changes the
// layout behind our back, e.g. a monitor that was turned off sleeps, drops off the bus and
// comes back, and Windows turns some display back on. When the stream reconnects, Steam
// removes its display and adds a new one; the watch follows it to the new display. While no
// virtual display exists Windows keeps a physical one on, and that is left alone.
func watchLayout(adapter luid, target uint32, stop chan struct{}) {
	t := time.NewTicker(watchPeriod)
	defer t.Stop()
	cur := targetRef{adapter, target}
	var last time.Time
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		if time.Since(last) < reisolateBackoff {
			continue
		}
		topo.Lock()
		select {
		case <-stop:
			topo.Unlock()
			return
		default:
		}
		virt, err := findVirtual()
		if err != nil || virt == nil {
			topo.Unlock()
			continue
		}
		keep := keepOutputs()
		if *virt != cur {
			cur = *virt
			paths, modes, _ := sessionLayout()
			setSessionLayout(paths, modes, &cur)
			last = time.Now()
			if err := isolate(cur.Adapter, cur.ID, keep); err != nil {
				log.Println("Steam's virtual display came back; turning the other displays off failed:", err)
			} else {
				log.Println("Steam's virtual display came back; other displays turned off again")
			}
		} else if drifted(cur.Adapter, cur.ID, keep) {
			last = time.Now()
			if err := isolate(cur.Adapter, cur.ID, keep); err != nil {
				log.Println("display layout changed during the session; re-isolating failed:", err)
			} else {
				log.Println("display layout changed during the session; virtual display isolated again")
			}
		}
		topo.Unlock()
	}
}

func (s *steamDisplay) destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop == nil {
		return nil
	}
	close(s.stop)
	<-s.done
	s.stop, s.done = nil, nil
	topo.Lock()
	defer topo.Unlock()
	if paths, modes, _ := sessionLayout(); paths != nil {
		if err := restore(paths, modes); err != nil {
			log.Println("could not restore the display layout:", err)
		}
	}
	setSessionLayout(nil, nil, nil)
	return nil
}
