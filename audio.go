package main

import (
	"errors"
	"fmt"
	"slices"
)

type role uint32

const (
	console role = iota
	multimedia
	communications
)

func (r role) String() string {
	switch r {
	case console:
		return "console"
	case multimedia:
		return "multimedia"
	case communications:
		return "communications"
	default:
		return fmt.Sprintf("role %d", r)
	}
}

type device struct {
	ID     string
	Name   string
	State  string
	Active bool
}

type audioClient interface {
	Devices() ([]device, error)
	Default(role) (string, error)
	SetDefault(string, role) error
	Close()
}

func nextTarget(targets []string, devices []device, current string) (device, error) {
	active := make(map[string]device)
	for _, d := range devices {
		if d.Active {
			active[d.ID] = d
		}
	}
	start := slices.Index(targets, current)
	for offset := 1; offset <= len(targets); offset++ {
		id := targets[(start+offset)%len(targets)]
		if d, ok := active[id]; ok && id != current {
			return d, nil
		}
	}
	return device{}, errors.New("no other configured output is available; connect a target or add another with 'audioswap add <id>'")
}

func switchOutput(audio audioClient, id string, includeComms bool) error {
	roles := []role{console, multimedia}
	if includeComms {
		roles = append(roles, communications)
	}
	previous := make([]string, len(roles))
	for i, r := range roles {
		value, err := audio.Default(r)
		if err != nil {
			return fmt.Errorf("read previous %s output: %w", r, err)
		}
		previous[i] = value
	}
	for i, r := range roles {
		if err := audio.SetDefault(id, r); err != nil {
			failures := []error{fmt.Errorf("set %s output: %w", r, err)}
			// A failing call may have applied the change before verification failed.
			for j := i; j >= 0; j-- {
				if previous[j] == "" {
					failures = append(failures, fmt.Errorf("cannot restore %s: there was no previous default", roles[j]))
					continue
				}
				if err := audio.SetDefault(previous[j], roles[j]); err != nil {
					failures = append(failures, fmt.Errorf("restore %s output: %w", roles[j], err))
				}
			}
			return errors.Join(failures...)
		}
	}
	return nil
}
