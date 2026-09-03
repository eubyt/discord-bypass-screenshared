package main

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
)

func TestGetTorBundleFilename(t *testing.T) {
	filename, err := getTorBundleFilename()
	if err != nil {
		t.Fatalf("getTorBundleFilename falhou: %v", err)
	}

	if !strings.HasPrefix(filename, "tor-expert-bundle-") {
		t.Errorf("Nome do arquivo inesperado: %s", filename)
	}

	if !strings.HasSuffix(filename, ".tar.gz") {
		t.Errorf("Extensão esperada .tar.gz, obtido: %s", filename)
	}
}

func TestGetTorCacheDir(t *testing.T) {
	dir := getTorCacheDir()
	if dir == "" {
		t.Fatal("Cache dir não pode ser vazio")
	}
	if !strings.Contains(dir, "discord_bypass_tor") {
		t.Errorf("Caminho do cache deve conter discord_bypass_tor, obtido: %s", dir)
	}
}

func TestIsSocks5Alive(t *testing.T) {
	// 1. Porta fechada -> deve retornar false
	if isSocks5Alive("127.0.0.1:59999") {
		t.Error("isSocks5Alive deveria retornar false para porta fechada")
	}

	// 2. Servidor Mock que responde como SOCKS5 válido
	lnValid, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Falha ao abrir listener mock: %v", err)
	}
	defer lnValid.Close()

	go func() {
		for {
			conn, err := lnValid.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 3)
			_, _ = io.ReadFull(conn, buf)
			// Responde SOCKS5: Version 5, Method 0 (Sem auth)
			_, _ = conn.Write([]byte{0x05, 0x00})
			conn.Close()
		}
	}()

	if !isSocks5Alive(lnValid.Addr().String()) {
		t.Error("isSocks5Alive deveria retornar true para resposta SOCKS5 válida")
	}

	// 3. Servidor Mock que responde lixo -> deve retornar false
	lnInvalid, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Falha ao abrir listener mock inválido: %v", err)
	}
	defer lnInvalid.Close()

	go func() {
		for {
			conn, err := lnInvalid.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte{0xFF, 0xFF})
			conn.Close()
		}
	}()

	if isSocks5Alive(lnInvalid.Addr().String()) {
		t.Error("isSocks5Alive deveria retornar false para resposta inválida")
	}
}

func TestProgressReader(t *testing.T) {
	data := []byte("hello world tor progress reader test data")
	pr := &progressReader{
		reader: bytes.NewReader(data),
		total:  int64(len(data)),
	}

	buf := make([]byte, 10)
	n, err := pr.Read(buf)
	if err != nil {
		t.Fatalf("Erro no read: %v", err)
	}
	if n != 10 {
		t.Errorf("Esperado 10 bytes, lido %d", n)
	}
	if pr.downloaded != 10 {
		t.Errorf("Esperado 10 downloaded, obtido %d", pr.downloaded)
	}
}
