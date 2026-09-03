package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func isDiscordRunning() bool {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq Discord*").Output()
		if err != nil {
			return false
		}
		low := strings.ToLower(string(out))
		return strings.Contains(low, "discord.exe") ||
			strings.Contains(low, "discordcanary.exe") ||
			strings.Contains(low, "discordptb.exe")
	default:
		err := exec.Command("pgrep", "-x", "Discord").Run()
		return err == nil
	}
}

func killDiscord() {
	if !isDiscordRunning() {
		return
	}

	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("taskkill", "/F", "/IM", "Discord.exe", "/T").Run()
		_ = exec.Command("taskkill", "/F", "/IM", "DiscordCanary.exe", "/T").Run()
		_ = exec.Command("taskkill", "/F", "/IM", "DiscordPTB.exe", "/T").Run()
	case "darwin":
		_ = exec.Command("pkill", "-TERM", "-x", "Discord").Run()
		_ = exec.Command("pkill", "-TERM", "-f", "Discord Helper").Run()
		for i := 0; i < 15; i++ {
			time.Sleep(200 * time.Millisecond)
			if !isDiscordRunning() {
				break
			}
		}
		if isDiscordRunning() {
			_ = exec.Command("pkill", "-KILL", "-x", "Discord").Run()
			_ = exec.Command("pkill", "-KILL", "-f", "Discord Helper").Run()
			time.Sleep(500 * time.Millisecond)
		}
	default: // Linux
		_ = exec.Command("pkill", "-TERM", "-x", "Discord").Run()
		time.Sleep(1 * time.Second)
		if isDiscordRunning() {
			_ = exec.Command("pkill", "-KILL", "-x", "Discord").Run()
		}
	}
}

func locateDiscord() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return locateDiscordWindows()
	case "darwin":
		return locateDiscordMac()
	default:
		return locateDiscordLinux()
	}
}

func locateDiscordWindows() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, _ := os.UserHomeDir()
		localAppData = filepath.Join(home, "AppData", "Local")
	}

	flavors := []string{"Discord", "DiscordCanary", "DiscordPTB"}
	for _, f := range flavors {
		root := filepath.Join(localAppData, f)
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}

		var latestApp string
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "app-") {
				if e.Name() > latestApp {
					latestApp = e.Name()
				}
			}
		}

		if latestApp != "" {
			exe := filepath.Join(root, latestApp, f+".exe")
			if _, err := os.Stat(exe); err == nil {
				return exe, nil
			}
			exeGeneric := filepath.Join(root, latestApp, "Discord.exe")
			if _, err := os.Stat(exeGeneric); err == nil {
				return exeGeneric, nil
			}
		}
	}

	programFiles := os.Getenv("ProgramFiles")
	if programFiles != "" {
		exe := filepath.Join(programFiles, "Discord", "Discord.exe")
		if _, err := os.Stat(exe); err == nil {
			return exe, nil
		}
	}

	return "", fmt.Errorf("Discord não encontrado no Windows")
}

func locateDiscordMac() (string, error) {
	home, _ := os.UserHomeDir()
	bundles := []string{
		"/Applications/Discord.app",
		"/Applications/Discord Canary.app",
		"/Applications/Discord PTB.app",
		filepath.Join(home, "Applications", "Discord.app"),
	}

	for _, bundle := range bundles {
		if _, err := os.Stat(bundle); err == nil {
			return bundle, nil
		}
	}

	out, err := exec.Command("mdfind", "kMDItemCFBundleIdentifier == 'com.hnc.Discord'").Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 && lines[0] != "" {
			return lines[0], nil
		}
	}

	return "", fmt.Errorf("Discord não encontrado no macOS")
}

func locateDiscordLinux() (string, error) {
	if _, err := exec.LookPath("flatpak"); err == nil {
		out, _ := exec.Command("flatpak", "list").Output()
		if strings.Contains(string(out), "com.discordapp.Discord") {
			return "flatpak:com.discordapp.Discord", nil
		}
	}

	if _, err := exec.LookPath("snap"); err == nil {
		if _, err := os.Stat("/snap/bin/discord"); err == nil {
			return "/snap/bin/discord", nil
		}
	}

	candidates := []string{
		"/usr/bin/discord",
		"/usr/bin/Discord",
		"/usr/local/bin/discord",
		"/opt/discord/Discord",
		"/opt/Discord/Discord",
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	if p, err := exec.LookPath("discord"); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("Discord não encontrado no Linux")
}

func launchDiscord(target, pacURL string) error {
	pacArg := "--proxy-pac-url=" + pacURL

	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command(target, pacArg)
		cmd.Dir = filepath.Dir(target)
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd.Start()

	case "darwin":
		appPath := target
		if idx := strings.Index(target, ".app"); idx != -1 {
			appPath = target[:idx+4]
		}
		cmd := exec.Command("open", "-a", appPath, "--args", pacArg)
		return cmd.Start()

	default: // Linux
		if strings.HasPrefix(target, "flatpak:") {
			appId := strings.TrimPrefix(target, "flatpak:")
			cmd := exec.Command("flatpak", "run", appId, pacArg)
			return cmd.Start()
		}
		cmd := exec.Command(target, pacArg)
		return cmd.Start()
	}
}
