package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	TorProxyPort        = "9050"
	TorProxyEndpoint    = "127.0.0.1:" + TorProxyPort
	TorInternalPort     = "9055"
	TorInternalEndpoint = "127.0.0.1:" + TorInternalPort
	TorProxyURL         = "socks5://" + TorProxyEndpoint
	TorVersion          = "15.0.21"
)

var TorMirrors = []string{
	"https://dist.torproject.org/torbrowser/" + TorVersion + "/",
	"https://tor.eff.org/dist/torbrowser/" + TorVersion + "/",
	"https://archive.torproject.org/tor-package-archive/torbrowser/" + TorVersion + "/",
}

var (
	torCmd        *exec.Cmd
	proxyListener net.Listener
)

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
	return filepath.Join(os.TempDir(), "discord_bypass_tor")
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

type progressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	lastPct    int
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.downloaded += int64(n)
	if pr.total > 0 {
		pct := int(float64(pr.downloaded) / float64(pr.total) * 100)
		if pct >= pr.lastPct+10 || pr.downloaded == pr.total {
			pr.lastPct = pct
			mbDown := float64(pr.downloaded) / 1024 / 1024
			mbTotal := float64(pr.total) / 1024 / 1024
			fmt.Printf("\r[*] Baixando Tor: %d%% (%.1f MB / %.1f MB)...", pct, mbDown, mbTotal)
			if pct >= 100 {
				fmt.Println()
			}
		}
	}
	return n, err
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

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
	}

	var resp *http.Response
	var respCancel context.CancelFunc
	var lastErr error

	for _, mirror := range TorMirrors {
		url := mirror + filename
		fmt.Printf("[*] Baixando Tor de: %s\n", mirror)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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

	pr := &progressReader{
		reader: resp.Body,
		total:  resp.ContentLength,
	}

	gzr, err := gzip.NewReader(pr)
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

	fmt.Println("[+] Tor portátil instalado com sucesso.")
	return torBinPath, nil
}

func extractTarget(buf []byte) string {
	if len(buf) < 7 || buf[0] != 0x05 {
		return ""
	}
	atyp := buf[3]
	switch atyp {
	case 0x01: // IPv4
		if len(buf) >= 10 {
			ip := net.IP(buf[4:8])
			port := binary.BigEndian.Uint16(buf[8:10])
			return fmt.Sprintf("%s:%d", ip.String(), port)
		}
	case 0x03: // Domain
		domainLen := int(buf[4])
		if len(buf) >= 5+domainLen+2 {
			domain := string(buf[5 : 5+domainLen])
			port := binary.BigEndian.Uint16(buf[5+domainLen : 5+domainLen+2])
			return fmt.Sprintf("%s:%d", domain, port)
		}
	case 0x04: // IPv6
		if len(buf) >= 22 {
			ip := net.IP(buf[4:20])
			port := binary.BigEndian.Uint16(buf[20:22])
			return fmt.Sprintf("[%s]:%d", ip.String(), port)
		}
	}
	return ""
}

func handleSocks5Connection(client net.Conn, torAddr string) {
	defer client.Close()

	torConn, err := net.DialTimeout("tcp", torAddr, 10*time.Second)
	if err != nil {
		return
	}
	defer torConn.Close()

	greeting := make([]byte, 258)
	n, err := client.Read(greeting)
	if err != nil || n < 3 {
		return
	}

	if _, err := torConn.Write(greeting[:n]); err != nil {
		return
	}

	torReply := make([]byte, 2)
	if _, err := io.ReadFull(torConn, torReply); err != nil {
		return
	}
	if _, err := client.Write(torReply); err != nil {
		return
	}

	reqBuf := make([]byte, 512)
	n, err = client.Read(reqBuf)
	if err != nil || n < 7 {
		return
	}

	target := extractTarget(reqBuf[:n])
	if target != "" {
		fmt.Printf("[TRAFEGO] Discord -> Tor: %s\n", target)
	}

	if _, err := torConn.Write(reqBuf[:n]); err != nil {
		return
	}

	replyHeader := make([]byte, 4)
	if _, err := io.ReadFull(torConn, replyHeader); err != nil {
		return
	}

	var addrLen int
	switch replyHeader[3] {
	case 0x01: // IPv4: 4 bytes IP + 2 bytes porta
		addrLen = 4 + 2
	case 0x04: // IPv6: 16 bytes IP + 2 bytes porta
		addrLen = 16 + 2
	case 0x03: // FQDN: 1 byte len + N bytes domínio + 2 bytes porta
		var l [1]byte
		if _, err := io.ReadFull(torConn, l[:]); err != nil {
			return
		}
		addrLen = int(l[0]) + 2
		replyHeader = append(replyHeader, l[0])
	default:
		return
	}

	addrRest := make([]byte, addrLen)
	if _, err := io.ReadFull(torConn, addrRest); err != nil {
		return
	}

	fullReply := append(replyHeader, addrRest...)
	if _, err := client.Write(fullReply); err != nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(torConn, client)
		if tcp, ok := torConn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(client, torConn)
		if tcp, ok := client.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()

	wg.Wait()
}

func startSocks5Logger(listenAddr, upstreamTorAddr string) error {
	var err error
	proxyListener, err = net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := proxyListener.Accept()
			if err != nil {
				return
			}
			go handleSocks5Connection(conn, upstreamTorAddr)
		}
	}()

	return nil
}

func stopTor() {
	if proxyListener != nil {
		_ = proxyListener.Close()
		proxyListener = nil
	}
	if torCmd != nil && torCmd.Process != nil {
		_ = torCmd.Process.Kill()
		_ = torCmd.Wait()
		torCmd = nil
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/IM", "tor.exe", "/T").Run()
	} else {
		_ = exec.Command("pkill", "-KILL", "-f", "discord_bypass_tor").Run()
		_ = exec.Command("pkill", "-KILL", "-x", "tor").Run()
	}
}

func ensureTorRunning() (string, error) {
	if isSocks5Alive(TorProxyEndpoint) {
		return TorProxyURL, nil
	}

	stopTor()
	time.Sleep(200 * time.Millisecond)

	torBin := findTorExecutable()
	if torBin == "" {
		var err error
		torBin, err = downloadTorBundle()
		if err != nil {
			return "", fmt.Errorf("não foi possível obter o Tor: %v", err)
		}
	}

	dataDir := filepath.Join(getTorCacheDir(), "data")
	_ = os.MkdirAll(dataDir, 0700)

	fmt.Println("[*] Iniciando Tor...")
	torCmd = exec.Command(torBin,
		"--SocksPort", TorInternalEndpoint,
		"--DataDirectory", dataDir,
		"--Log", "notice stdout",
	)

	stdoutPipe, err := torCmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("falha ao conectar no stdout do Tor: %v", err)
	}

	if err := torCmd.Start(); err != nil {
		return "", fmt.Errorf("falha ao executar tor: %v", err)
	}

	readyCh := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		lastPct := ""
		for scanner.Scan() {
			line := scanner.Text()
			if idx := strings.Index(line, "Bootstrapped "); idx != -1 {
				parts := strings.Split(line[idx:], " ")
				if len(parts) >= 2 {
					pct := parts[1]
					if pct != lastPct {
						lastPct = pct
						fmt.Printf("\r[*] Conectando à rede Tor: %s...", pct)
					}
				}
			}
			if strings.Contains(line, "Bootstrapped 100%") {
				fmt.Println()
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
			return "", fmt.Errorf("processo do Tor encerrou inesperadamente")
		}
	case <-time.After(45 * time.Second):
		if !isSocks5Alive(TorInternalEndpoint) {
			stopTor()
			return "", fmt.Errorf("tempo limite esgotado aguardando da rede Tor")
		}
	}

	fmt.Println("[+] Rede Tor conectada (100%).")

	if err := startSocks5Logger(TorProxyEndpoint, TorInternalEndpoint); err != nil {
		stopTor()
		return "", fmt.Errorf("falha ao iniciar logger na porta %s: %v", TorProxyPort, err)
	}

	return TorProxyURL, nil
}
