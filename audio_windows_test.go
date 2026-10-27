package main

import (
	"os"
	"testing"
)

func TestWindowsAudio(t *testing.T) {
	if os.Getenv("AUDIOSWAP_INTEGRATION") != "1" {
		t.Skip("set AUDIOSWAP_INTEGRATION=1 to test real Windows audio APIs")
	}
	audio, err := openAudio()
	if err != nil {
		t.Fatal(err)
	}
	defer audio.Close()
	devices, err := audio.Devices()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Enumerated %d Windows output endpoints", len(devices))
	active := make(map[string]bool)
	for _, d := range devices {
		if d.Active {
			active[d.ID] = true
		}
	}
	for _, r := range []role{console, multimedia, communications} {
		id, err := audio.Default(r)
		if err != nil {
			t.Fatal(err)
		}
		if id == "" {
			t.Logf("No default %s output to reapply", r)
			continue
		}
		if !active[id] {
			t.Fatalf("%s default is not an active render endpoint", r)
		}
		if err := audio.SetDefault(id, r); err != nil {
			t.Fatalf("reapply existing %s default: %v", r, err)
		}
		t.Logf("Reapplied and verified existing %s default", r)
	}
}
