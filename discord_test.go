package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestBypassListContent(t *testing.T) {
	requiredBypasses := []string{
		"<local>",
		"127.0.0.1",
		"localhost",
		"cdn.discordapp.com",
		"*.discordapp.net",
		"*.discord.media",
		"*.voice.discord.media",
		"dealer.spotify.com",
		"*.spotify.com",
		"*.storage.googleapis.com",
	}

	for _, req := range requiredBypasses {
		if !strings.Contains(BypassList, req) {
			t.Errorf("BypassList deve conter %s", req)
		}
	}

	forbiddenBypasses := []string{
		"gateway.discord.gg",
		"discord.com;",
	}

	for _, forb := range forbiddenBypasses {
		if strings.Contains(BypassList, forb) {
			t.Errorf("BypassList NÃO deve conter %s (deve passar pelo Tor)", forb)
		}
	}
}

func TestLocateDiscord(t *testing.T) {
	path, err := locateDiscord()
	if runtime.GOOS == "darwin" {
		// No Mac do usuário o Discord está instalado
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
