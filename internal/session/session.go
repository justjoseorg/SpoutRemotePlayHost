// Package session ties Remote Play sessions to the virtual monitor lifecycle.
package session

import (
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

func New(cfg *config.Store, disp display.Manager) *Controller {
	return &Controller{cfg: cfg, disp: disp, Grace: 5 * time.Second}
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

// Start handles a Remote Play session starting for device id ("" when unknown).
// It reports whether a monitor was created.
func (c *Controller) Start(id string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancelLocked()
	cfg := c.cfg.GetFor(id)
	if !cfg.AutoCreate {
		return false, nil
	}
	if c.disp.Active() && c.owner == id && c.mode == modeOf(cfg) {
		return false, nil
	}
	if err := c.createLocked(id); err != nil {
		return false, err
	}
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
		_ = c.disp.Destroy()
		return
	}
	c.timer = time.AfterFunc(c.Grace, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.timer != nil {
			c.timer = nil
			_ = c.disp.Destroy()
		}
	})
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
