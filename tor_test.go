package main

import (
	"os"
	"strings"
	"testing"
)

func TestGetTorBundleFilename(t *testing.T) {
	filename, err := getTorBundleFilename()
	if err != nil {
		t.Fatalf("Erro ao obter nome do bundle: %v", err)
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
