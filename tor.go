package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	TorPort     = "9050"
	TorEndpoint = "127.0.0.1:" + TorPort
	TorProxyURL = "socks5://" + TorEndpoint
	TorVersion  = "15.0.21"
)

var TorMirrors = []string{
	"https://dist.torproject.org/torbrowser/" + TorVersion + "/",
	"https://tor.eff.org/dist/torbrowser/" + TorVersion + "/",
	"https://archive.torproject.org/tor-package-archive/torbrowser/" + TorVersion + "/",
}

var torCmd *exec.Cmd

func isSocks5Alive(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 800*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(1 * time.Second))
	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		return false
	}

	reply := make([]byte, 2)
	n, err := io.ReadFull(conn, reply)
	if err != nil || n < 2 {
		return false
	}
	return reply[0] == 0x05 && reply[1] == 0x00
}

func getTorCacheDir() string {
	return filepath.Join(os.TempDir(), "discord_tor")
}

func findTorExecutable() string {
	binaryName := "tor"
	if runtime.GOOS == "windows" {
		binaryName = "tor.exe"
	}

	cacheDir := getTorCacheDir()
	candidates := []string{
		filepath.Join(cacheDir, "tor", binaryName),
		filepath.Join(cacheDir, binaryName),
		filepath.Join(".", "tor", binaryName),
		filepath.Join(".", binaryName),
	}

	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}

	if p, err := exec.LookPath(binaryName); err == nil {
		return p
	}

	return ""
}

func getTorBundleFilename() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" || runtime.GOARCH == "386" {
			return fmt.Sprintf("tor-expert-bundle-windows-x86_64-%s.tar.gz", TorVersion), nil
		}
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return fmt.Sprintf("tor-expert-bundle-macos-aarch64-%s.tar.gz", TorVersion), nil
		}
		return fmt.Sprintf("tor-expert-bundle-macos-x86_64-%s.tar.gz", TorVersion), nil
	case "linux":
		if runtime.GOARCH == "amd64" {
			return fmt.Sprintf("tor-expert-bundle-linux-x86_64-%s.tar.gz", TorVersion), nil
		}
	}
	return "", fmt.Errorf("sistema ou arquitetura não suportada: %s/%s", runtime.GOOS, runtime.GOARCH)
}

func downloadTorBundle() (string, error) {
	filename, err := getTorBundleFilename()
	if err != nil {
		return "", err
	}

	destDir := getTorCacheDir()
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	var resp *http.Response
	var respCancel context.CancelFunc
	var lastErr error

	for _, mirror := range TorMirrors {
		url := mirror + filename
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}

		res, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}

		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			cancel()
			lastErr = fmt.Errorf("HTTP %d", res.StatusCode)
			continue
		}

		resp = res
		respCancel = cancel
		break
	}

	if resp == nil {
		return "", fmt.Errorf("todos os mirrors falharam: %v", lastErr)
	}
	defer respCancel()
	defer resp.Body.Close()

	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var torBinPath string

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		targetPath := filepath.Join(destDir, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(targetPath, 0755)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)|0755)
			if err != nil {
				continue
			}
			_, _ = io.Copy(outFile, tr)
			outFile.Close()

			base := filepath.Base(hdr.Name)
			if base == "tor" || base == "tor.exe" {
				torBinPath = targetPath
			}
		}
	}

	if torBinPath == "" {
		return "", fmt.Errorf("binário do tor não encontrado no arquivo")
	}

	if runtime.GOOS == "darwin" {
		_ = exec.Command("xattr", "-cr", destDir).Run()
		dylibPath := filepath.Join(filepath.Dir(torBinPath), "libevent-2.1.7.dylib")
		if _, err := os.Stat(dylibPath); err == nil {
			_ = exec.Command("codesign", "--force", "-s", "-", dylibPath).Run()
		}
		_ = exec.Command("codesign", "--force", "-s", "-", torBinPath).Run()
	}

	return torBinPath, nil
}

func stopTor() {
	if torCmd != nil && torCmd.Process != nil {
		_ = torCmd.Process.Kill()
		_ = torCmd.Wait()
		torCmd = nil
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/IM", "tor.exe", "/T").Run()
	} else {
		_ = exec.Command("pkill", "-KILL", "-f", "discord_tor").Run()
		_ = exec.Command("pkill", "-KILL", "-x", "tor").Run()
	}
}

// EnsureTorRunning garante que o Tor esteja ativo e pronto para uso
func EnsureTorRunning() (string, error) {
	if isSocks5Alive(TorEndpoint) {
		return TorProxyURL, nil
	}

	stopTor()
	time.Sleep(200 * time.Millisecond)

	torBin := findTorExecutable()
	if torBin == "" {
		fmt.Println("[*] Baixando Tor portátil para /tmp...")
		var err error
		torBin, err = downloadTorBundle()
		if err != nil {
			return "", fmt.Errorf("não foi possível obter o Tor: %w", err)
		}
	}

	dataDir := filepath.Join(getTorCacheDir(), "data")
	_ = os.MkdirAll(dataDir, 0700)

	fmt.Println("[*] Iniciando Tor, aguarde...")
	torCmd = exec.Command(torBin,
		"--SocksPort", TorEndpoint,
		"--DataDirectory", dataDir,
		"--Log", "notice stdout",
	)

	stdoutPipe, err := torCmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("falha no stdout do Tor: %w", err)
	}

	if err := torCmd.Start(); err != nil {
		return "", fmt.Errorf("falha ao executar Tor: %w", err)
	}

	readyCh := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "Bootstrapped 100%") {
				readyCh <- true
				return
			}
		}
		readyCh <- false
	}()

	select {
	case ok := <-readyCh:
		if !ok {
			stopTor()
			return "", fmt.Errorf("Tor encerrou inesperadamente")
		}
	case <-time.After(35 * time.Second):
		if !isSocks5Alive(TorEndpoint) {
			stopTor()
			return "", fmt.Errorf("tempo limite esgotado conectando ao Tor")
		}
	}

	return TorProxyURL, nil
}
