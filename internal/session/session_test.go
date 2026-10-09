package session

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/display"
)

type fake struct {
	active         bool
	creates, kills int
	last           display.Mode
}

func (f *fake) Create(m display.Mode) error { f.active, f.last = true, m; f.creates++; return nil }
func (f *fake) Destroy() error              { f.active = false; f.kills++; return nil }
func (f *fake) Active() bool                { return f.active }

func setup(t *testing.T) (*Controller, *fake, *config.Store) {
	st, err := config.Open(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fake{}
	c := New(st, f)
	c.Grace = 0
	return c, f, st
}

func TestStartUsesDeviceConfigAndIsIdempotent(t *testing.T) {
	c, f, st := setup(t)
	m := st.GetFor("dev")
	m.Width, m.Height, m.RefreshHz = 1280, 800, 90
	if err := st.SetFor("dev", m); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.Start("dev"); !ok || err != nil || f.last != (display.Mode{Width: 1280, Height: 800, RefreshHz: 90}) {
		t.Fatalf("%v %v %+v", ok, err, f.last)
	}
	if ok, _ := c.Start("dev"); ok || f.creates != 1 {
		t.Fatal("same session should not recreate the monitor")
	}
	if ok, _ := c.Start(""); !ok || f.last != (display.Mode{Width: 1920, Height: 1080, RefreshHz: 60}) || f.creates != 2 {
		t.Fatalf("different device should switch: %+v", f.last)
	}
}

func TestStartRespectsAutoCreateOff(t *testing.T) {
	c, f, st := setup(t)
	m := st.GetFor("")
	m.AutoCreate = false
	st.SetFor("", m)
	if ok, _ := c.Start(""); ok || f.active {
		t.Fatal("autoCreate off must not create")
	}
}

func TestStopOwnerOnly(t *testing.T) {
	c, f, _ := setup(t)
	c.Start("a")
	c.Stop("b")
	if !f.active {
		t.Fatal("other device stop must not destroy")
	}
	c.Stop("a")
	if f.active {
		t.Fatal("owner stop should destroy")
	}
	c.Start("a")
	c.Stop("")
	if f.active {
		t.Fatal("unknown id stops whatever is active")
	}
}

func TestGraceKeepsMonitorAcrossRestart(t *testing.T) {
	c, f, _ := setup(t)
	c.Grace = 80 * time.Millisecond
	c.Start("a")
	c.Stop("a")
	time.Sleep(20 * time.Millisecond)
	c.Start("a")
	time.Sleep(150 * time.Millisecond)
	if !f.active || f.kills != 0 {
		t.Fatalf("restart within grace should keep monitor: %+v", f)
	}
	c.Stop("a")
	time.Sleep(200 * time.Millisecond)
	if f.active {
		t.Fatal("monitor should be destroyed after grace")
	}
}
