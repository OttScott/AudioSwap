package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDatabaseRoundTripAndReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audioswap.json")
	db, err := loadDatabase(path)
	if err != nil || len(db.Targets) != 0 {
		t.Fatalf("missing file: got %v, %v", db, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("reading an absent database must not create it")
	}
	for _, targets := range [][]string{{"headphones", "speakers"}, {"speakers"}, {}} {
		want := database{Targets: targets}
		if err := saveDatabase(path, want); err != nil {
			t.Fatal(err)
		}
		got, err := loadDatabase(path)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, %v; want %v", got, err, want)
		}
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".audioswap-*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary files were left behind: %v, %v", leftovers, err)
	}
}

func TestDatabaseRejectsInvalidInputWithoutChangingIt(t *testing.T) {
	inputs := []string{
		``,
		`{`,
		`null`,
		`{}`,
		`{"targets":null}`,
		`{"targets":"speakers"}`,
		`{"targets":["speakers","speakers"]}`,
		`{"targets":[" "]}`,
		`{"targets":["bad\u0000id"]}`,
		`{"targets":[],"unknown":true}`,
		`{"targets":[]} {"targets":[]}`,
		`{"targets":[]} trailing`,
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "audioswap.json")
			if err := os.WriteFile(path, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadDatabase(path); err == nil {
				t.Fatal("expected malformed input to fail")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != input {
				t.Fatalf("invalid database changed: %q, %v", got, err)
			}
		})
	}
}

func TestDatabaseRejectsInvalidSaveWithoutOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audioswap.json")
	want := database{Targets: []string{"speakers"}}
	if err := saveDatabase(path, want); err != nil {
		t.Fatal(err)
	}
	if err := saveDatabase(path, database{Targets: []string{"duplicate", "duplicate"}}); err == nil {
		t.Fatal("expected invalid save to fail")
	}
	got, err := loadDatabase(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("original targets changed: %v, %v", got, err)
	}
}
