BUILD_DIR = build
TARGET = $(BUILD_DIR)/discord-tor
TARGET_WIN = $(BUILD_DIR)/discord-tor.exe

.PHONY: all build build-windows run test status clean

all: build

build:
	@mkdir -p $(BUILD_DIR)
	@echo "[*] Compilando para $(shell uname -s)..."
	go build -ldflags="-s -w" -o $(TARGET) .
	@echo "[+] Gerado: $(TARGET)"

build-windows:
	@mkdir -p $(BUILD_DIR)
	@echo "[*] Compilando para Windows (amd64)..."
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(TARGET_WIN) .
	@echo "[+] Gerado: $(TARGET_WIN)"

run: build
	@./$(TARGET)

test:
	@echo "[*] Executando testes unitários..."
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)
