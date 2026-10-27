package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCLIConfigLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audioswap.json")
	audio := newFakeAudio()
	open := func() (audioClient, error) { return audio, nil }
	var out bytes.Buffer
	for _, args := range [][]string{{"devices"}, {"add", "speakers"}, {"add", "offline"}, {"targets"}} {
		if err := run(args, &out, path, open); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, want := range []string{"USB headphones", "unplugged", "1. speakers", "2. offline", path} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "speakers"}, {"add", "unknown"}, {"remove", "unknown"}} {
		if err := run(args, &out, path, open); err == nil {
			t.Fatalf("%v: expected error", args)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed commands changed the database: %s, %v", after, err)
	}
	noAudio := func() (audioClient, error) {
		t.Fatal("remove/targets must not require Windows audio")
		return nil, nil
	}
	for _, args := range [][]string{{"remove", "speakers"}, {"remove", "offline"}, {"targets"}} {
		if err := run(args, &out, path, noAudio); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	db, err := loadDatabase(path)
	if err != nil || len(db.Targets) != 0 {
		t.Fatalf("got %v, %v; want empty rotation", db, err)
	}
}

func TestCLIRotatesFromActualDefault(t *testing.T) {
	for _, includeComms := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "include communications"}[includeComms], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "audioswap.json")
			if err := saveDatabase(path, database{Targets: []string{"speakers", "offline", "headphones"}}); err != nil {
				t.Fatal(err)
			}
			audio := newFakeAudio()
			open := func() (audioClient, error) { return audio, nil }
			args := []string{}
			if includeComms {
				args = []string{"--include-comms"}
			}
			var out bytes.Buffer
			for _, want := range []string{"headphones", "speakers"} {
				if err := run(args, &out, path, open); err != nil {
					t.Fatal(err)
				}
				if audio.defaults[console] != want || audio.defaults[multimedia] != want {
					t.Fatalf("did not rotate to %q: %v", want, audio.defaults)
				}
				if includeComms && audio.defaults[communications] != want {
					t.Fatalf("did not rotate communications: %v", audio.defaults)
				}
				if !includeComms && audio.defaults[communications] != "calls" {
					t.Fatal("normal swap changed communications")
				}
			}
			if !audio.closed {
				t.Fatal("audio client was not closed")
			}
		})
	}
}

func TestCLINoAlternativeDoesNotChangeDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audioswap.json")
	if err := saveDatabase(path, database{Targets: []string{"speakers", "offline"}}); err != nil {
		t.Fatal(err)
	}
	audio := newFakeAudio()
	var out bytes.Buffer
	if err := run(nil, &out, path, func() (audioClient, error) { return audio, nil }); err == nil {
		t.Fatal("expected error when only the current output is available")
	}
	if len(audio.changes) != 0 || out.Len() != 0 {
		t.Fatalf("no alternative caused a change or success output: %v, %s", audio.changes, out.String())
	}
}

func TestCLIRejectsBadArgumentsBeforeOpeningAudio(t *testing.T) {
	for _, args := range [][]string{
		{"unknown"}, {"add"}, {"remove"}, {"devices", "extra"},
		{"--unknown"}, {"--include-comms", "devices"}, {"devices", "--include-comms"},
		{"add", "one", "two"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			err := run(args, &out, filepath.Join(t.TempDir(), "audioswap.json"), func() (audioClient, error) {
				t.Fatal("bad arguments must not open audio")
				return nil, nil
			})
			if err == nil {
				t.Fatal("expected argument error")
			}
		})
	}
}

func TestCLIMissingConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audioswap.json")
	for _, args := range [][]string{nil, {"targets"}, {"help"}, {"--help"}} {
		var out bytes.Buffer
		err := run(args, &out, path, func() (audioClient, error) {
			t.Fatal("empty rotation, listing targets, and help must not open audio")
			return nil, nil
		})
		if len(args) == 0 && err == nil {
			t.Fatal("swap without targets must fail")
		}
		if len(args) != 0 && err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("read-only commands created configuration")
	}
}

func TestCLIAudioErrorsAreReportedWithoutChangingConfiguration(t *testing.T) {
	for _, failOpen := range []bool{true, false} {
		path := filepath.Join(t.TempDir(), "audioswap.json")
		want := database{Targets: []string{"speakers", "headphones"}}
		if err := saveDatabase(path, want); err != nil {
			t.Fatal(err)
		}
		audio := newFakeAudio()
		audio.devicesErr = errors.New("enumeration failed")
		var out bytes.Buffer
		err := run(nil, &out, path, func() (audioClient, error) {
			if failOpen {
				return nil, errors.New("COM failed")
			}
			return audio, nil
		})
		if err == nil || out.Len() != 0 {
			t.Fatalf("got %v, %s; want error without success output", err, out.String())
		}
		got, readErr := loadDatabase(path)
		if readErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("configuration changed: %v, %v", got, readErr)
		}
	}
}
