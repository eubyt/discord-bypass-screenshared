package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestLocateDiscord(t *testing.T) {
	path, err := locateDiscord()
	if runtime.GOOS == "darwin" {
		if err != nil {
			t.Fatalf("Discord deveria ser localizado no macOS: %v", err)
		}
		if !strings.Contains(path, "Discord") {
			t.Errorf("Caminho retornado não contém Discord: %s", path)
		}
	}
}

func TestIsDiscordRunning(t *testing.T) {
	_ = isDiscordRunning()
}
