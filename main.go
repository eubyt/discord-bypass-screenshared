package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func waitOnExit() {
	fmt.Print("\nPressione Enter para sair...")
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
}

func printBanner() {
	fmt.Println("============================================================")
	fmt.Println("  Bypass do Bloqueio do Discord (Brasil) • por Eubyt")
	fmt.Println("============================================================")
}

func main() {
	if runtime.GOOS == "windows" {
		// Ajusta o título do console no Windows
		_ = syscall.Exec
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		printBanner()
		fmt.Println("[1] Usar Rede Tor (Automático)")
		fmt.Println("[2] Usar Proxy / VPN personalizada")
		fmt.Println("[0] Sair")
		fmt.Print("\nEscolha uma opção: ")

		input, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		choice := strings.TrimSpace(input)
		switch choice {
		case "0":
			fmt.Println("Encerrando.")
			return

		case "1":
			runBypassTor(reader)
			return

		case "2":
			if runBypassCustom(reader) {
				return
			}
			// Se o teste falhar na opção 2, volta para o menu
			fmt.Println()

		default:
			fmt.Println("[-] Opção inválida. Tente novamente.")
		}
	}
}

func runBypassTor(reader *bufio.Reader) {
	port, err := EnsureTorRunning()
	if err != nil {
		fmt.Printf("[-] Erro ao iniciar o Tor: %v\n", err)
		waitOnExit()
		return
	}

	pacReturn := fmt.Sprintf("SOCKS5 127.0.0.1:%d", port)
	startSession(pacReturn, true)
}

func runBypassCustom(reader *bufio.Reader) bool {
	fmt.Print("\nDigite o endereço da Proxy / VPN (ex: 127.0.0.1:1080 ou socks5://...): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	cfg, err := ParseProxyAddress(input)
	if err != nil {
		fmt.Printf("[-] Endereço inválido: %v\n", err)
		return false
	}

	fmt.Printf("[*] Testando conexão via %s...\n", cfg.RawURL)
	info, err := TestTunnelReal(cfg.RawURL)
	if err != nil {
		fmt.Printf("[-] Teste de túnel falhou: %v\n", err)
		fmt.Println("[!] O Discord NÃO foi fechado. Verifique os dados e tente novamente.")
		return false
	}

	fmt.Printf("[+] Túnel validado com sucesso: %s\n", info)
	startSession(cfg.PacReturn, false)
	return true
}

func startSession(pacReturn string, isTor bool) {
	// 1. Inicia o mini-servidor PAC na porta dinâmica :0
	pacScript := BuildPacScript(pacReturn)
	pacServer, pacURL, err := StartPacServer(pacScript)
	if err != nil {
		fmt.Printf("[-] Erro ao iniciar servidor PAC: %v\n", err)
		if isTor {
			stopTor()
		}
		waitOnExit()
		return
	}
	defer func() {
		_ = pacServer.Shutdown(context.Background())
	}()

	fmt.Printf("[+] Servidor PAC ativo em %s\n", pacURL)

	// 2. Localiza o Discord
	discordPath, err := locateDiscord()
	if err != nil {
		fmt.Printf("[-] %v\n", err)
		if isTor {
			stopTor()
		}
		waitOnExit()
		return
	}
	fmt.Printf("[+] Discord localizado: %s\n", discordPath)

	// 3. Fecha instâncias anteriores do Discord
	if isDiscordRunning() {
		fmt.Println("[*] Fechando instância anterior do Discord...")
		killDiscord()
		time.Sleep(1 * time.Second)
	}

	// 4. Inicia o Discord apontando para o PAC
	fmt.Println("[*] Aplicando bypass de Gateway e iniciando Discord...")
	if err := launchDiscord(discordPath, pacURL); err != nil {
		fmt.Printf("[-] Falha ao iniciar Discord: %v\n", err)
		if isTor {
			stopTor()
		}
		waitOnExit()
		return
	}

	fmt.Println("[+] Discord iniciado com bypass de rede ativo!")
	fmt.Println("[*] Monitorando sessão (Pressione Ctrl+C para encerrar)...")

	// 5. Canal de sinais (Ctrl+C / SIGTERM)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Aguarda o processo do Discord aparecer
	for i := 0; i < 30; i++ {
		if isDiscordRunning() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Loop de supervisão
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	closedByDiscord := false

	for {
		select {
		case <-sigCh:
			fmt.Println("\n[*] Sinal recebido. Encerrando Discord e túnel...")
			killDiscord()
			if isTor {
				stopTor()
			}
			fmt.Println("[+] Sessão encerrada com sucesso.")
			return

		case <-ticker.C:
			if !isDiscordRunning() {
				// Confirma ausência em 2 checagens consecutivas
				time.Sleep(1 * time.Second)
				if !isDiscordRunning() {
					closedByDiscord = true
					break
				}
			}
		}

		if closedByDiscord {
			break
		}
	}

	fmt.Println("\n[*] Discord foi encerrado pelo usuário.")
	if isTor {
		stopTor()
	}
	_ = pacServer.Shutdown(context.Background())

	waitOnExit()
}
