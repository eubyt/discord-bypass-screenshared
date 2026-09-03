package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

func waitOnWindows() {
	if runtime.GOOS == "windows" {
		fmt.Println("\nPressione Enter para fechar esta janela...")
		var b [1]byte
		_, _ = os.Stdin.Read(b[:])
	}
}

func main() {
	if runtime.GOOS == "windows" {
		fmt.Print("\033]0;Discord Bypass\007")
	}

	fmt.Println("============================================================")
	fmt.Println("  Bypass do Bloqueio do Discord no Brasil")
	fmt.Println("  Desenvolvido por Eubyt")
	fmt.Println("============================================================")

	killOnly := flag.Bool("k", false, "Fecha o Discord e o Tor")
	testOnly := flag.Bool("t", false, "Testa a conexão Tor")
	statusOnly := flag.Bool("s", false, "Verifica o status atual")
	flag.Parse()

	if *killOnly {
		fmt.Println("[*] Fechando Discord e Tor...")
		killDiscord()
		stopTor()
		fmt.Println("[+] Processos finalizados.")
		waitOnWindows()
		return
	}

	if *statusOnly {
		torOnline := isSocks5Alive(TorProxyEndpoint) || isSocks5Alive(TorInternalEndpoint)
		discordOnline := isDiscordRunning()
		fmt.Printf("Tor:     %v\n", torOnline)
		fmt.Printf("Discord: %v\n", discordOnline)
		waitOnWindows()
		return
	}

	// Tor
	fmt.Println("[*] Verificando Tor...")
	torProxy, err := ensureTorRunning()
	if err != nil {
		fmt.Printf("[-] Erro no Tor: %v\n", err)
		waitOnWindows()
		os.Exit(1)
	}
	fmt.Printf("[+] Tor pronto em %s\n", torProxy)

	if *testOnly {
		testTorConnection(torProxy)
		stopTor()
		waitOnWindows()
		return
	}

	// Discord
	fmt.Println("[*] Localizando Discord...")
	discordPath, err := locateDiscord()
	if err != nil {
		fmt.Printf("[-] Erro: %v\n", err)
		stopTor()
		waitOnWindows()
		os.Exit(1)
	}
	fmt.Printf("[+] Discord encontrado: %s\n", discordPath)

	if isDiscordRunning() {
		fmt.Println("[*] Fechando instância anterior...")
		killDiscord()
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("[*] Aplicando bypass de rede e iniciando Discord...")
	if err := launchDiscord(discordPath, torProxy); err != nil {
		fmt.Printf("[-] Falha ao iniciar: %v\n", err)
		stopTor()
		waitOnWindows()
		os.Exit(1)
	}
	fmt.Println("[+] Discord iniciado com bypass de rede ativo.")
	fmt.Println("[*] Monitorando conexões (Pressione Ctrl+C para encerrar)...")
	fmt.Println("------------------------------------------------------------")

	// Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	discordClosed := make(chan struct{})
	go func() {
		// Aguarda o processo do Discord
		started := false
		for i := 0; i < 30; i++ {
			time.Sleep(1 * time.Second)
			if isDiscordRunning() {
				started = true
				break
			}
		}

		if !started {
			fmt.Println("\n[-] AVISO: O processo do Discord não iniciou em 30s.")
			close(discordClosed)
			return
		}

		// Monitora até o Discord fechar de fato
		misses := 0
		for {
			time.Sleep(1 * time.Second)
			if !isDiscordRunning() {
				misses++
				if misses >= 3 {
					close(discordClosed)
					return
				}
			} else {
				misses = 0
			}
		}
	}()

	select {
	case <-sigChan:
		fmt.Println("\n[*] Sinal recebido (Ctrl+C). Encerrando Discord e Tor...")
	case <-discordClosed:
		fmt.Println("\n[*] Discord foi fechado pelo usuário. Encerrando Tor...")
	}

	killDiscord()
	stopTor()
	fmt.Println("[+] Discord e Tor finalizados com sucesso.")
	waitOnWindows()
}

func testTorConnection(proxyURLStr string) {
	fmt.Println("[*] Testando IP de saída do Tor...")
	proxyURL, err := url.Parse(proxyURLStr)
	if err != nil {
		fmt.Printf("[-] URL inválida: %v\n", err)
		return
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 20 * time.Second,
	}

	resp, err := client.Get("https://check.torproject.org/api/ip")
	if err != nil {
		fmt.Printf("[-] Falha no teste: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[+] Resposta: %s\n", string(body))
}
