package steamlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	ev, ok := Parse("[2026-10-09 11:32:03][4482.484212] Streaming started to ayn-odin-2-portal at 192.168.1.174:60366, audio channels = 2, MTU = 1468")
	if !ok || !ev.Start || ev.Client != "ayn-odin-2-portal" {
		t.Fatalf("%+v %v", ev, ok)
	}
	if ev, ok := Parse("[x][y] PipeWire: Deinitializing streaming"); !ok || ev.Start {
		t.Fatal("stop not detected")
	}
	if _, ok := Parse(">>> Stopped desktop stream"); ok {
		t.Fatal("desktop stream restarts are not session ends")
	}
}

func TestWatchTailsNewLinesOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "streaming_log.txt")
	os.WriteFile(p, []byte("Streaming started to old at 1.2.3.4:5, x\n"), 0o644)
	got := make(chan Event, 4)
	stop := make(chan struct{})
	defer close(stop)
	go Watch([]string{p}, 10*time.Millisecond, stop, func(e Event) { got <- e })
	time.Sleep(60 * time.Millisecond)
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("Streaming started to deck at 1.2.3.4:5, x\nPipeWire: Deinit")
	time.Sleep(60 * time.Millisecond)
	f.WriteString("ializing streaming\n")
	f.Close()
	for i, want := range []Event{{true, "deck"}, {false, ""}} {
		select {
		case e := <-got:
			if e != want {
				t.Fatalf("%d: %+v", i, e)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%d: timeout", i)
		}
	}
}
