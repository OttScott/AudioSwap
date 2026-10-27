package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type database struct {
	Targets []string `json:"targets"`
}

func (db database) validate() error {
	if db.Targets == nil {
		return errors.New("'targets' must be an array of output device IDs")
	}
	seen := make(map[string]bool)
	for _, id := range db.Targets {
		if strings.TrimSpace(id) == "" || strings.ContainsRune(id, '\x00') {
			return errors.New("target IDs must not be blank or contain NUL characters")
		}
		if seen[id] {
			return fmt.Errorf("duplicate target ID %q", id)
		}
		seen[id] = true
	}
	return nil
}

func loadDatabase(path string) (database, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return database{Targets: []string{}}, nil
	}
	if err != nil {
		return database{}, fmt.Errorf("read targets file %q: %w", path, err)
	}
	var db database
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&db); err != nil {
		return database{}, fmt.Errorf("parse targets file %q: %w", path, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return database{}, fmt.Errorf("targets file %q must contain exactly one JSON object", path)
	}
	if err := db.validate(); err != nil {
		return database{}, fmt.Errorf("invalid targets file %q: %w", path, err)
	}
	return db, nil
}

func saveDatabase(path string, db database) (err error) {
	if err := db.validate(); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".audioswap-*.tmp")
	if err != nil {
		return fmt.Errorf("create targets file beside %q: %w", path, err)
	}
	tempPath := file.Name()
	defer func() {
		if removeErr := os.Remove(tempPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("clean up temporary targets file: %w", removeErr))
		}
	}()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(db)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("write targets file %q: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace targets file %q: %w", path, err)
	}
	return nil
}
