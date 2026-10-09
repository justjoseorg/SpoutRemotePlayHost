package display

import "testing"

type fake struct{ creates, destroys int }

func (f *fake) create(Mode) error { f.creates++; return nil }
func (f *fake) destroy() error    { f.destroys++; return nil }

func TestManagerRecreateAndDestroy(t *testing.T) {
	f := &fake{}
	m := newWithBackend(f)
	_ = m.Create(Mode{1280, 800, 60})
	_ = m.Create(Mode{1920, 1080, 60})
	if f.creates != 2 || f.destroys != 1 || !m.Active() {
		t.Fatalf("%+v active=%v", f, m.Active())
	}
	_ = m.Destroy()
	_ = m.Destroy()
	if f.destroys != 2 || m.Active() {
		t.Fatalf("%+v", f)
	}
}

func TestStubReportsNotImplemented(t *testing.T) {
	m := newWithBackend(stub{})
	if err := m.Create(Mode{1280, 800, 60}); err != ErrNotImplemented || m.Active() {
		t.Fatal("stub should fail and stay inactive")
	}
}
