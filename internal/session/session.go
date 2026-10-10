// Package session ties Remote Play sessions to the virtual monitor lifecycle.
package session

import (
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/display"
)

// Controller creates the virtual monitor from the connecting device's config and
// removes it shortly after the session ends.
type Controller struct {
	cfg  *config.Store
	disp display.Manager
	// Grace delays destruction so a stream that restarts immediately keeps its monitor.
	Grace time.Duration

	mu    sync.Mutex
	owner string
	mode  display.Mode
	timer *time.Timer
}

// A dropped stream can take Steam about a minute to reconnect. On Windows the physical displays
// come back on by themselves as soon as Steam removes its virtual display, so waiting longer
// only delays putting the exact layout back; elsewhere the host's own monitor would linger.
func defaultGrace() time.Duration {
	if runtime.GOOS == "windows" {
		return 90 * time.Second
	}
	return 5 * time.Second
}

func New(cfg *config.Store, disp display.Manager) *Controller {
	return &Controller{cfg: cfg, disp: disp, Grace: defaultGrace()}
}

func modeOf(m config.Monitor) display.Mode {
	return display.Mode{Width: m.Width, Height: m.Height, RefreshHz: m.RefreshHz}
}

// Create makes the monitor for a device unconditionally (manual create from the UI).
func (c *Controller) Create(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.createLocked(id)
}

func (c *Controller) createLocked(id string) error {
	c.cancelLocked()
	m := modeOf(c.cfg.GetFor(id))
	if err := c.disp.Create(m); err != nil {
		return err
	}
	c.owner, c.mode = id, m
	return nil
}

func (c *Controller) cancelLocked() {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

// Destroy removes the monitor immediately.
func (c *Controller) Destroy() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancelLocked()
	return c.disp.Destroy()
}

// Start handles a Remote Play session starting for device id ("" when the client isn't paired).
// Only paired devices get a virtual monitor, on every platform. It reports whether one was created.
func (c *Controller) Start(id string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id == "" {
		log.Println("session from an unpaired client: no virtual monitor")
		return false, nil
	}
	if c.timer != nil && c.disp.Active() && c.owner == id {
		log.Printf("session for device %q resumed before its monitor was removed", id)
	}
	c.cancelLocked()
	cfg := c.cfg.GetFor(id)
	if !cfg.AutoCreate {
		log.Printf("session for device %q: auto-create is off", id)
		return false, nil
	}
	if c.disp.Active() && c.owner == id && c.mode == modeOf(cfg) {
		return false, nil
	}
	if err := c.createLocked(id); err != nil {
		return false, err
	}
	log.Printf("virtual monitor %dx%d@%d created for device %q", c.mode.Width, c.mode.Height, c.mode.RefreshHz, id)
	return true, nil
}

// Stop handles the session ending. An unknown id ("") stops whatever is active.
func (c *Controller) Stop(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.disp.Active() || (id != "" && c.owner != id) {
		return
	}
	c.cancelLocked()
	if c.Grace <= 0 {
		c.destroyLocked()
		return
	}
	c.timer = time.AfterFunc(c.Grace, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.timer != nil {
			c.timer = nil
			c.destroyLocked()
		}
	})
}

func (c *Controller) destroyLocked() {
	if err := c.disp.Destroy(); err != nil {
		log.Println("virtual monitor:", err)
		return
	}
	log.Printf("session ended: virtual monitor for device %q removed", c.owner)
}

// OwnedBy reports whether the live monitor was created for device id.
func (c *Controller) OwnedBy(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disp.Active() && c.owner == id
}

// Reapply re-creates the live monitor with id's current config.
func (c *Controller) Reapply(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.createLocked(id)
}
