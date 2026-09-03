package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestGetTorBundleFilename(t *testing.T) {
	filename, hash, err := getTorBundleFilename()
	if err != nil {
		t.Fatalf("Erro ao obter nome do bundle: %v", err)
	}

	if len(hash) != 64 {
		t.Errorf("Hash SHA-256 esperado com 64 chars, obtido: %s", hash)
	}

	if !strings.HasPrefix(filename, "tor-expert-bundle-") || !strings.HasSuffix(filename, ".tar.gz") {
		t.Errorf("Nome de arquivo inesperado: %s", filename)
	}
}

func TestGetTorCacheDir(t *testing.T) {
	dir := getTorCacheDir()
	if !strings.HasPrefix(dir, os.TempDir()) {
		t.Errorf("Diretório de cache deve ficar em os.TempDir: %s", dir)
	}
}

func TestEnsureTorRunning(t *testing.T) {
	port, err := EnsureTorRunning()
	if err != nil {
		t.Fatalf("EnsureTorRunning falhou: %v", err)
	}
	defer stopTor()

	if !torEntregando(port, 5*time.Second) {
		t.Errorf("torEntregando falhou na porta %d", port)
	}
}
