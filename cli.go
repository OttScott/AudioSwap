package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
)

const usage = `audioswap - rotate Windows audio outputs

Usage:
  audioswap                     Switch to the next available configured output
  audioswap --include-comms     Also switch the communications/calls default
  audioswap devices             List Windows output devices and their IDs
  audioswap targets             Show the configured rotation and file location
  audioswap add "<id>"          Append an output to the rotation
  audioswap remove "<id>"       Remove an output from the rotation
  audioswap help                Show this help

Targets are stored in audioswap.json beside the executable, not in the current
working directory. Add order is rotation order. Normal swaps change the console
and multimedia defaults, leaving communications unchanged. Unavailable targets
are skipped. If the current multimedia output is not in the list, the first
available configured output is selected.
`

func run(args []string, out io.Writer, path string, open func() (audioClient, error)) error {
	flags := flag.NewFlagSet("audioswap", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	includeComms := flags.Bool("include-comms", false, "")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, err = io.WriteString(out, usage)
			return err
		}
		return fmt.Errorf("%w; use 'audioswap help' for usage", err)
	}
	args = flags.Args()
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	wantArgs := 1
	switch command {
	case "":
		wantArgs = 0
	case "devices", "targets", "help":
	case "add", "remove":
		wantArgs = 2
	default:
		return fmt.Errorf("unknown command %q; use 'audioswap help' for usage", command)
	}
	if len(args) != wantArgs {
		return fmt.Errorf("incorrect arguments for %q; use 'audioswap help' for usage", command)
	}
	if command != "" && flags.NFlag() != 0 {
		return errors.New("--include-comms applies only to a swap, without a subcommand")
	}
	if command == "help" {
		_, err := io.WriteString(out, usage)
		return err
	}
	var db database
	if command != "devices" {
		var err error
		db, err = loadDatabase(path)
		if err != nil {
			return err
		}
	}
	switch command {
	case "targets":
		if _, err := fmt.Fprintf(out, "Targets file: %s\n", path); err != nil {
			return err
		}
		if len(db.Targets) == 0 {
			_, err := fmt.Fprintln(out, "No targets configured. Use 'audioswap devices', then 'audioswap add <id>'.")
			return err
		}
		for i, id := range db.Targets {
			if _, err := fmt.Fprintf(out, "%d. %s\n", i+1, id); err != nil {
				return err
			}
		}
		return nil
	case "remove":
		index := slices.Index(db.Targets, args[1])
		if index == -1 {
			return fmt.Errorf("target %q is not configured", args[1])
		}
		db.Targets = slices.Delete(db.Targets, index, index+1)
		if err := saveDatabase(path, db); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Removed %s\n", args[1])
		return err
	case "add":
		if slices.Contains(db.Targets, args[1]) {
			return fmt.Errorf("target %q is already configured", args[1])
		}
	case "":
		if len(db.Targets) == 0 {
			return errors.New("no targets configured; use 'audioswap devices', then 'audioswap add <id>'")
		}
	}
	audio, err := open()
	if err != nil {
		return fmt.Errorf("open Windows audio: %w", err)
	}
	defer audio.Close()
	devices, err := audio.Devices()
	if err != nil {
		return fmt.Errorf("list output devices: %w", err)
	}
	switch command {
	case "devices":
		return printDevices(out, devices, audio)
	case "add":
		for _, d := range devices {
			if d.ID != args[1] {
				continue
			}
			db.Targets = append(db.Targets, d.ID)
			if err := saveDatabase(path, db); err != nil {
				return err
			}
			_, err := fmt.Fprintf(out, "Added %s\n%s\n", d.Name, d.ID)
			return err
		}
		return fmt.Errorf("output ID %q was not found; copy an ID from 'audioswap devices'", args[1])
	default:
		current, err := audio.Default(multimedia)
		if err != nil {
			return fmt.Errorf("read current multimedia output: %w", err)
		}
		next, err := nextTarget(db.Targets, devices, current)
		if err != nil {
			return err
		}
		if err := switchOutput(audio, next.ID, *includeComms); err != nil {
			return err
		}
		scope := "console and multimedia"
		if *includeComms {
			scope += " and communications"
		}
		_, err = fmt.Fprintf(out, "Switched %s output to %s\n%s\n", scope, next.Name, next.ID)
		return err
	}
}

func printDevices(out io.Writer, devices []device, audio audioClient) error {
	defaults := make(map[string][]string)
	for _, r := range []role{console, multimedia, communications} {
		id, err := audio.Default(r)
		if err != nil {
			return fmt.Errorf("read %s default: %w", r, err)
		}
		if id != "" {
			defaults[id] = append(defaults[id], r.String())
		}
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "NAME\tSTATE\tDEFAULT FOR\tID"); err != nil {
		return err
	}
	for _, d := range devices {
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", d.Name, d.State, strings.Join(defaults[d.ID], ", "), d.ID); err != nil {
			return err
		}
	}
	return table.Flush()
}
