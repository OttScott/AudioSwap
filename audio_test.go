package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestNextTarget(t *testing.T) {
	devices := []device{
		{ID: "speakers", Active: true},
		{ID: "headphones", Active: true},
		{ID: "monitor", Active: true},
		{ID: "offline"},
	}
	tests := []struct {
		name    string
		targets []string
		current string
		want    string
	}{
		{"next", []string{"speakers", "headphones", "monitor"}, "speakers", "headphones"},
		{"wrap", []string{"speakers", "headphones", "monitor"}, "monitor", "speakers"},
		{"skip unavailable", []string{"speakers", "offline", "removed", "headphones"}, "speakers", "headphones"},
		{"unconfigured default", []string{"offline", "headphones", "speakers"}, "monitor", "headphones"},
		{"no default", []string{"speakers", "headphones"}, "", "speakers"},
		{"one different target", []string{"headphones"}, "speakers", "headphones"},
		{"only current available", []string{"speakers", "offline"}, "speakers", ""},
		{"all unavailable", []string{"offline", "removed"}, "speakers", ""},
		{"empty rotation", nil, "speakers", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nextTarget(tt.targets, devices, tt.current)
			if tt.want == "" {
				if err == nil {
					t.Fatal("expected an error, not a successful no-op")
				}
			} else if err != nil || got.ID != tt.want {
				t.Fatalf("got %q, %v; want %q", got.ID, err, tt.want)
			}
		})
	}
}

type defaultChange struct {
	id   string
	role role
}

type fakeAudio struct {
	devices    []device
	defaults   map[role]string
	changes    []defaultChange
	defaultErr error
	devicesErr error
	failChange func(string, role) error
	closed     bool
}

func (a *fakeAudio) Devices() ([]device, error) {
	return a.devices, a.devicesErr
}

func (a *fakeAudio) Default(r role) (string, error) {
	return a.defaults[r], a.defaultErr
}

func (a *fakeAudio) SetDefault(id string, r role) error {
	a.changes = append(a.changes, defaultChange{id, r})
	a.defaults[r] = id
	if a.failChange != nil {
		return a.failChange(id, r)
	}
	return nil
}

func (a *fakeAudio) Close() {
	a.closed = true
}

func newFakeAudio() *fakeAudio {
	return &fakeAudio{
		devices: []device{
			{ID: "speakers", Name: "Desk speakers", State: "active", Active: true},
			{ID: "headphones", Name: "USB headphones", State: "active", Active: true},
			{ID: "offline", Name: "Disconnected monitor", State: "unplugged"},
		},
		defaults: map[role]string{
			console:        "speakers",
			multimedia:     "speakers",
			communications: "calls",
		},
	}
}

func TestSwitchOutputRoles(t *testing.T) {
	for _, includeComms := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "include communications"}[includeComms], func(t *testing.T) {
			audio := newFakeAudio()
			err := switchOutput(audio, "headphones", includeComms)
			if err != nil {
				t.Fatal(err)
			}
			want := []defaultChange{{"headphones", console}, {"headphones", multimedia}}
			if includeComms {
				want = append(want, defaultChange{"headphones", communications})
			}
			if !reflect.DeepEqual(audio.changes, want) {
				t.Fatalf("changes = %v; want %v", audio.changes, want)
			}
			if !includeComms && audio.defaults[communications] != "calls" {
				t.Fatal("normal swap changed communications")
			}
		})
	}
}

func TestSwitchOutputRestoresDefaultsAfterFailure(t *testing.T) {
	audio := newFakeAudio()
	audio.defaults[console] = "original-console"
	audio.failChange = func(id string, r role) error {
		if id == "headphones" && r == communications {
			return errors.New("policy failure")
		}
		return nil
	}
	err := switchOutput(audio, "headphones", true)
	if err == nil || !strings.Contains(err.Error(), "policy failure") {
		t.Fatalf("got %v; want the switch failure", err)
	}
	want := map[role]string{console: "original-console", multimedia: "speakers", communications: "calls"}
	if !reflect.DeepEqual(audio.defaults, want) {
		t.Fatalf("defaults not restored: %v", audio.defaults)
	}
}

func TestSwitchOutputReportsRestorationFailure(t *testing.T) {
	audio := newFakeAudio()
	audio.failChange = func(id string, r role) error {
		if id == "headphones" && r == multimedia {
			return errors.New("switch failed")
		}
		if id == "speakers" && r == console {
			return errors.New("restore failed")
		}
		return nil
	}
	err := switchOutput(audio, "headphones", false)
	if err == nil || !strings.Contains(err.Error(), "switch failed") || !strings.Contains(err.Error(), "restore failed") {
		t.Fatalf("got %v; want both failures", err)
	}
}

func TestSwitchOutputReadFailureDoesNotChangeDefaults(t *testing.T) {
	audio := newFakeAudio()
	audio.defaultErr = errors.New("cannot read default")
	if err := switchOutput(audio, "headphones", true); err == nil {
		t.Fatal("expected error")
	}
	if len(audio.changes) != 0 {
		t.Fatalf("changed defaults despite read failure: %v", audio.changes)
	}
}
