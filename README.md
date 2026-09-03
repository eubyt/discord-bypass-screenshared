# Discord Bypass Screenshared

O Discord Bypass é uma ferramenta multiplataforma desenvolvida em Go para contornar bloqueios de conexão do Discord por meio de roteamento seletivo pela rede Tor.

## 1. O que é e Como Funciona

O programa atua como um **supervisor de execução e proxy de controle** para o cliente desktop do Discord (Electron/Chromium). Em vez de jogar todo o tráfego cegamente pelo Tor ele aplica uma estratégia de **roteamento seletivo**.

- **Roteamento Seletivo no Chromium**:
  O Discord é inicializado injetando switches nativos de rede:
  - `--proxy-server=socks5://127.0.0.1:9050`: Direciona autenticação, API REST e conexões em tempo real (WebSocket Gateway) para o Tor.
  - `--proxy-bypass-list=...`: Força conexão direta pela internet local para:
    - **Voz e Vídeo WebRTC (`*.discord.media`, `*.voice.discord.media`)**: O Tor suporta apenas TCP; o áudio e a transmissão de tela usam pacotes UDP de baixa latência (<50ms).

- **Proxy Intermediário em Go (SOCKS5 Sniffer)**:
  - Escuta em `127.0.0.1:9050` e repassa os fluxos para a porta interna do Tor (`127.0.0.1:9055`).

## 2. Como Usar

### Execução Padrão

Inicia o Tor, abre o Discord com o bypass ativo e exibe o tráfego ao vivo:

```bash
./build/discord-tor
```

_(No Windows, basta dar um **duplo clique** em `discord-tor.exe`)._

Exemplo de saída no terminal:

```text
[*] Verificando Tor...
[*] Iniciando Tor...
[+] Rede Tor conectada (100%).
[+] Tor pronto em socks5://127.0.0.1:9050
[*] Localizando Discord...
[+] Discord encontrado: /Applications/Discord.app
[*] Fechando instância anterior...
[*] Aplicando bypass de rede e iniciando Discord...
[+] Discord iniciado com bypass de rede ativo.
[*] Monitorando conexões (Pressione Ctrl+C para encerrar)...
------------------------------------------------------------
[TRAFEGO] Discord -> Tor: discord.com:443
[TRAFEGO] Discord -> Tor: gateway.discord.gg:443
[TRAFEGO] Discord -> Tor: status.discord.com:443
```

Para encerrar o Discord e o Tor juntos, pressione `Ctrl+C` no terminal ou feche a janela do Discord.

---

### Opções de Linha de Comando (Flags)

| Flag             | Descrição                                                                        |
| :--------------- | :------------------------------------------------------------------------------- |
| _(sem flag)_     | Modo padrão: sobe o Tor, aplica as regras de rede no Discord e monitora a sessão |
| `discord-tor -s` | **Status**: Verifica se o Tor e o Discord estão em execução sem alterar nada     |
| `discord-tor -t` | **Teste**: Testa a rota e exibe o endereço IP do nó de saída do Tor              |
| `discord-tor -k` | **Kill**: Força o encerramento imediato de instâncias ativas do Discord e do Tor |

---

## 3. Como Compilar (Build)

Todos os binários gerados são salvos na pasta `build`.

```bash
# 1. Compilar para o sistema atual (macOS ou Linux)
make build

# 2. Compilar para Windows
make build-windows

# 3. Executar os testes unitários automatizados
make test

# 4. Compilar e rodar imediatamente
make run

# 5. Limpar os artefatos gerados
make clean
```
