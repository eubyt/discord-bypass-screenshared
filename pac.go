package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProxyConfig armazena os detalhes da proxy validada
type ProxyConfig struct {
	RawURL    string // ex: "socks5://127.0.0.1:1080" ou "http://1.2.3.4:8080"
	PacReturn string // ex: "SOCKS5 127.0.0.1:1080" ou "PROXY 1.2.3.4:8080"
}

// ParseProxyAddress normaliza entradas do usuário para uma URL válida e formato PAC
func ParseProxyAddress(input string) (*ProxyConfig, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil, fmt.Errorf("endereço de proxy vazio")
	}

	// Se o usuário não digitou esquema, assume socks5 por padrão
	if !strings.Contains(s, "://") {
		s = "socks5://" + s
	}

	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("formato de proxy inválido: %s", input)
	}

	hostPort := u.Host
	if !strings.Contains(hostPort, ":") {
		return nil, fmt.Errorf("porta não informada (ex: 127.0.0.1:1080)")
	}

	scheme := strings.ToLower(u.Scheme)
	var pacReturn string

	switch scheme {
	case "socks5", "socks5h", "socks4":
		pacReturn = "SOCKS5 " + hostPort
	case "http", "https":
		pacReturn = "PROXY " + hostPort
	default:
		return nil, fmt.Errorf("esquema não suportado: %s (use socks5:// ou http://)", u.Scheme)
	}

	return &ProxyConfig{
		RawURL:    u.String(),
		PacReturn: pacReturn,
	}, nil
}

// BuildPacScript gera o script PAC que roteia estritamente gateway.discord.gg
func BuildPacScript(pacReturn string) string {
	return fmt.Sprintf(`function FindProxyForURL(url, host) {
    if (shExpMatch(host, "gateway.discord.gg")) {
        return "%s; DIRECT";
    }
    return "DIRECT";
}
`, pacReturn)
}

// StartPacServer inicia o mini-servidor HTTP local na porta dinâmica :0
func StartPacServer(pacContent string) (*http.Server, string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("falha ao abrir listener PAC: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/pac", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(pacContent))
	})

	server := &http.Server{
		Handler: mux,
	}

	go func() {
		_ = server.Serve(listener)
	}()

	addr := listener.Addr().String()
	pacURL := fmt.Sprintf("http://%s/pac", addr)
	return server, pacURL, nil
}

// TestTunnelReal realiza o handshake TLS duplo através do túnel:
// 1. cloudflare.com/cdn-cgi/trace (testa túnel, handshake TLS, IP e país fora do Brasil)
// 2. gateway.discord.gg (testa se o gateway do Discord responde através da rota)
func TestTunnelReal(rawProxyURL string) (string, error) {
	proxyParsed, err := url.Parse(rawProxyURL)
	if err != nil {
		return "", fmt.Errorf("proxy inválida: %w", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyParsed),
		DialContext: (&net.Dialer{
			Timeout:   8 * time.Second,
			KeepAlive: 15 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 8 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   12 * time.Second,
	}

	// 1. Teste Cloudflare cdn-cgi/trace
	reqTrace, err := http.NewRequestWithContext(context.Background(), "GET", "https://cloudflare.com/cdn-cgi/trace", nil)
	if err != nil {
		return "", err
	}
	reqTrace.Header.Set("User-Agent", "Mozilla/5.0")

	respTrace, err := client.Do(reqTrace)
	if err != nil {
		return "", fmt.Errorf("falha no túnel TLS (cloudflare.com): %w", err)
	}
	defer respTrace.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(respTrace.Body, 4096))
	bodyStr := string(bodyBytes)

	// Extrai país e IP do trace
	var ip, loc string
	lines := strings.Split(bodyStr, "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "ip=") {
			ip = strings.TrimPrefix(l, "ip=")
		} else if strings.HasPrefix(l, "loc=") {
			loc = strings.TrimPrefix(l, "loc=")
		}
	}

	if loc == "BR" {
		return "", fmt.Errorf("o IP de saída (%s) é do Brasil (loc=BR). Use uma rota internacional", ip)
	}

	// 2. Teste direto ao gateway.discord.gg
	reqGateway, err := http.NewRequestWithContext(context.Background(), "GET", "https://gateway.discord.gg", nil)
	if err != nil {
		return "", err
	}
	reqGateway.Header.Set("User-Agent", "Mozilla/5.0")

	respGateway, err := client.Do(reqGateway)
	if err != nil {
		return "", fmt.Errorf("gateway.discord.gg inalcançável através da proxy: %w", err)
	}
	defer respGateway.Body.Close()

	return fmt.Sprintf("IP=%s País=%s (Gateway acessível)", ip, loc), nil
}
