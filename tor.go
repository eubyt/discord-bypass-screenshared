package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
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
	TorBundleVersion = "13.5"
	TorDedicatedPort = 9060
	TorBaseURL       = "https://archive.torproject.org/tor-package-archive/torbrowser/" + TorBundleVersion + "/"
)

// TorHashes contém os hashes oficiais SHA-256 de cada tarball do Tor 13.5
var TorHashes = map[string]string{
	"tor-expert-bundle-linux-x86_64-13.5.tar.gz":   "147158f33c5f2c539d58d8fab69ca5af384778e7bbae951fbc7ac8ca58ac4e0d",
	"tor-expert-bundle-windows-x86_64-13.5.tar.gz": "5978ccc2a7fed783c329474888e87f5e6349aa132d9c43016418bff296c7becb",
	"tor-expert-bundle-macos-aarch64-13.5.tar.gz":  "e18f749fbe6114c918735e950b28c1f476a5c9d8bf224f5ec26e6bffa1222d49",
	"tor-expert-bundle-macos-x86_64-13.5.tar.gz":   "9e23c21a4e45dc45b599e723373530ef7cabef106367b43677a534fae099b10d",
}

// Portas onde um Tor pode estar atendendo, na ordem de preferência:
// 1. Nossa porta dedicada 9060
// 2. Tor do sistema (9050)
// 3. Tor Browser (9150)
// 4. Outras portas comuns (9250, 9052)
var TorCandidatePorts = []int{TorDedicatedPort, 9050, 9150, 9250, 9052}

var torCmd *exec.Cmd

// isPortAlive faz uma checagem rápida TCP de 400ms na porta
func isPortAlive(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 400*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// torEntregando testa se o SOCKS5 na porta consegue abrir um túnel até gateway.discord.gg:443
func torEntregando(port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), timeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	// 1. Saudação SOCKS5 sem autenticação
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return false
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return false
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		return false
	}

	// 2. SOCKS5 CONNECT até gateway.discord.gg:443
	host := "gateway.discord.gg"
	req := make([]byte, 0, 7+len(host))
	req = append(req, 0x05, 0x01, 0x00, 0x03, byte(len(host)))
	req = append(req, []byte(host)...)
	req = append(req, 0x01, 0xbb) // porta 443 (0x01BB)

	if _, err := conn.Write(req); err != nil {
		return false
	}

	resp := make([]byte, 4)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return false
	}

	// resp[1] == 0x00 significa que o túnel foi estabelecido com sucesso até o gateway!
	return resp[0] == 0x05 && resp[1] == 0x00
}

func getTorCacheDir() string {
	return filepath.Join(os.TempDir(), "discord_tor")
}

func getTorBundleFilename() (string, string, error) {
	var filename string
	switch runtime.GOOS {
	case "windows":
		filename = fmt.Sprintf("tor-expert-bundle-windows-x86_64-%s.tar.gz", TorBundleVersion)
	case "darwin":
		if runtime.GOARCH == "arm64" {
			filename = fmt.Sprintf("tor-expert-bundle-macos-aarch64-%s.tar.gz", TorBundleVersion)
		} else {
			filename = fmt.Sprintf("tor-expert-bundle-macos-x86_64-%s.tar.gz", TorBundleVersion)
		}
	case "linux":
		filename = fmt.Sprintf("tor-expert-bundle-linux-x86_64-%s.tar.gz", TorBundleVersion)
	default:
		return "", "", fmt.Errorf("sistema operacional não suportado: %s", runtime.GOOS)
	}

	expectedHash, ok := TorHashes[filename]
	if !ok {
		return "", "", fmt.Errorf("hash SHA-256 desconhecido para %s", filename)
	}

	return filename, expectedHash, nil
}

func findTorBinary(dir string) string {
	binaryName := "tor"
	if runtime.GOOS == "windows" {
		binaryName = "tor.exe"
	}

	candidates := []string{
		filepath.Join(dir, "tor", binaryName),
		filepath.Join(dir, binaryName),
	}

	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

func downloadAndExtractTor() (string, error) {
	filename, expectedHash, err := getTorBundleFilename()
	if err != nil {
		return "", err
	}

	destDir := getTorCacheDir()
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}

	tarballPath := filepath.Join(destDir, filename)
	url := TorBaseURL + filename

	fmt.Printf("[*] Baixando Tor Expert Bundle %s (~25 MB)...\n", TorBundleVersion)
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("falha ao baixar Tor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download retornou HTTP %d", resp.StatusCode)
	}

	hasher := sha256.New()
	outFile, err := os.OpenFile(tarballPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return "", err
	}

	tee := io.TeeReader(resp.Body, hasher)
	if _, err := io.Copy(outFile, tee); err != nil {
		outFile.Close()
		return "", err
	}
	outFile.Close()

	// Validação SHA-256 estrita
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHash, expectedHash) {
		_ = os.Remove(tarballPath)
		return "", fmt.Errorf("hash SHA-256 não confere (esperado %s, obtido %s)", expectedHash, actualHash)
	}
	fmt.Println("[+] SHA-256 verificado com sucesso.")

	// Extração
	fmt.Println("[*] Extraindo pacote do Tor...")
	archiveFile, err := os.Open(tarballPath)
	if err != nil {
		return "", err
	}
	defer archiveFile.Close()

	gzr, err := gzip.NewReader(archiveFile)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
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
			f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)|0755)
			if err != nil {
				continue
			}
			_, _ = io.Copy(f, tr)
			f.Close()
		}
	}

	_ = os.Remove(tarballPath)

	binPath := findTorBinary(destDir)
	if binPath == "" {
		return "", fmt.Errorf("binário do tor não encontrado após extração")
	}

	if runtime.GOOS == "darwin" {
		_ = exec.Command("xattr", "-cr", destDir).Run()
		_ = exec.Command("codesign", "--force", "-s", "-", binPath).Run()
	}

	return binPath, nil
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

// EnsureTorRunning garante que haja um Tor rodando e entregando túnel ao Gateway
func EnsureTorRunning() (int, error) {
	// 1. Checa se já existe um Tor atendendo e entregando em alguma das portas candidatas
	for _, port := range TorCandidatePorts {
		if isPortAlive(port) && torEntregando(port, 2*time.Second) {
			fmt.Printf("[+] Tor detectado e respondendo na porta %d.\n", port)
			return port, nil
		}
	}

	// 2. Se a porta 9060 estiver travada sem entregar, encerra processos órfãos
	stopTor()
	time.Sleep(300 * time.Millisecond)

	destDir := getTorCacheDir()
	binPath := findTorBinary(destDir)
	if binPath == "" {
		var err error
		binPath, err = downloadAndExtractTor()
		if err != nil {
			return 0, fmt.Errorf("falha ao preparar Tor: %w", err)
		}
	}

	dataDir := filepath.Join(destDir, "data-state")
	_ = os.MkdirAll(dataDir, 0700)

	geoip := filepath.Join(destDir, "data", "geoip")
	geoip6 := filepath.Join(destDir, "data", "geoip6")

	torrcPath := filepath.Join(destDir, "torrc")
	torrcContent := fmt.Sprintf("SocksPort %d\nDataDirectory %s\n", TorDedicatedPort, dataDir)
	if fi, err := os.Stat(geoip); err == nil && !fi.IsDir() {
		torrcContent += fmt.Sprintf("GeoIPFile %s\n", geoip)
	}
	if fi, err := os.Stat(geoip6); err == nil && !fi.IsDir() {
		torrcContent += fmt.Sprintf("GeoIPv6File %s\n", geoip6)
	}
	torrcContent += "Log notice stdout\n"

	_ = os.WriteFile(torrcPath, []byte(torrcContent), 0644)

	fmt.Println("[*] Iniciando Tor automático na porta 9060, aguarde...")
	torCmd = exec.Command(binPath, "-f", torrcPath)

	if runtime.GOOS == "linux" {
		torCmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+filepath.Dir(binPath))
	} else if runtime.GOOS == "darwin" {
		torCmd.Env = append(os.Environ(), "DYLD_LIBRARY_PATH="+filepath.Dir(binPath))
	}

	stdoutPipe, err := torCmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("falha no stdout do Tor: %w", err)
	}

	if err := torCmd.Start(); err != nil {
		return 0, fmt.Errorf("falha ao executar Tor: %w", err)
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
			return 0, fmt.Errorf("Tor encerrou inesperadamente")
		}
	case <-time.After(40 * time.Second):
		if !torEntregando(TorDedicatedPort, 2*time.Second) {
			stopTor()
			return 0, fmt.Errorf("tempo limite esgotado para bootstrap do Tor")
		}
	}

	// Validação final do túnel SOCKS5 até gateway.discord.gg:443
	for attempt := 1; attempt <= 3; attempt++ {
		if torEntregando(TorDedicatedPort, 8*time.Second) {
			fmt.Println("[+] Túnel SOCKS5 verificado com sucesso até gateway.discord.gg:443!")
			return TorDedicatedPort, nil
		}
		time.Sleep(1 * time.Second)
	}

	stopTor()
	return 0, fmt.Errorf("o Tor subiu mas recusou conexão até gateway.discord.gg:443")
}
