//go:build !windows

package main

import "errors"

func openAudio() (audioClient, error) {
	return nil, errors.New("audioswap supports Windows only")
}
