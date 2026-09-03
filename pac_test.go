package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBuildPacScript(t *testing.T) {
	proxy := "SOCKS5 127.0.0.1:9050"
	pac := BuildPacScript(proxy)

	if !strings.Contains(pac, `shExpMatch(host, "gateway.discord.gg")`) {
		t.Errorf("PAC deve conter regra para gateway.discord.gg")
	}

	if !strings.Contains(pac, proxy) {
		t.Errorf("PAC deve conter o proxy configurado: %s", proxy)
	}

	if !strings.Contains(pac, `return "DIRECT";`) {
		t.Errorf("PAC deve retornar DIRECT para todo o resto")
	}

	// Não deve conter regras globais ou discord.com inteiro
	if strings.Contains(pac, `shExpMatch(host, "discord.com")`) {
		t.Errorf("PAC NÃO deve rotear discord.com inteiro")
	}
}

func TestParseProxyAddress(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		expectedPac string
	}{
		{
			name:        "SOCKS5 sem esquema",
			input:       "127.0.0.1:1080",
			wantErr:     false,
			expectedPac: "SOCKS5 127.0.0.1:1080",
		},
		{
			name:        "SOCKS5 com esquema explícito",
			input:       "socks5://192.168.1.50:9050",
			wantErr:     false,
			expectedPac: "SOCKS5 192.168.1.50:9050",
		},
		{
			name:        "HTTP Proxy",
			input:       "http://proxy.corporativo.com:8080",
			wantErr:     false,
			expectedPac: "PROXY proxy.corporativo.com:8080",
		},
		{
			name:        "SOCKS5 com autenticação",
			input:       "socks5://user:pass@127.0.0.1:1080",
			wantErr:     false,
			expectedPac: "SOCKS5 127.0.0.1:1080",
		},
		{
			name:    "Entrada vazia",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Porta ausente",
			input:   "127.0.0.1",
			wantErr: true,
		},
		{
			name:    "Esquema não suportado",
			input:   "ftp://127.0.0.1:21",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseProxyAddress(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseProxyAddress(%q) erro = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && cfg.PacReturn != tt.expectedPac {
				t.Errorf("PacReturn = %q, esperado = %q", cfg.PacReturn, tt.expectedPac)
			}
		})
	}
}

func TestStartPacServer(t *testing.T) {
	pacContent := BuildPacScript("SOCKS5 127.0.0.1:9050")
	server, pacURL, err := StartPacServer(pacContent)
	if err != nil {
		t.Fatalf("Falha ao iniciar StartPacServer: %v", err)
	}
	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	if !strings.HasPrefix(pacURL, "http://127.0.0.1:") || !strings.HasSuffix(pacURL, "/pac") {
		t.Errorf("URL do PAC inesperada: %s", pacURL)
	}

	resp, err := http.Get(pacURL)
	if err != nil {
		t.Fatalf("Erro ao requisitar PAC: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status esperado 200, recebido: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/x-ns-proxy-autoconfig") {
		t.Errorf("Content-Type esperado PAC, recebido: %s", contentType)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != pacContent {
		t.Errorf("Conteúdo retornado diverge do script PAC original")
	}
}
