# Discord Bypass

Utilitário leve em Go para contornar o bloqueio nacional do Discord e liberar o **Go Live (compartilhamento de tela)** e a **câmera** para usuários brasileiros.

---

## 1. O que é e Como Funciona

O Discord não bloqueia a transmissão de tela pelo cliente. Ele decide se você pode transmitir baseado no IP de onde parte a conexão do WebSocket de autenticação (`gateway.discord.gg`). Se a conexão for do Brasil, a conta recebe a trava. Se o gateway for conectado a partir de um IP internacional, a trava é removida.

### Detalhes da Arquitetura:

- **Mini-Servidor PAC Interno Dinâmico**:
  O programa abre um mini-servidor HTTP local em uma porta dinâmica livre (`127.0.0.1:0`), servindo um script PAC (_Proxy Auto-Config_):
  ```javascript
  function FindProxyForURL(url, host) {
    if (shExpMatch(host, "gateway.discord.gg")) {
      return "%PROXY%; DIRECT";
    }
    return "DIRECT";
  }
  ```
- **Lançamento Nativo via `--proxy-pac-url`**:
  O Discord é iniciado passando a URL do PAC. O Chromium aplica a regra automaticamente:
  - **Apenas `gateway.discord.gg`** passa pelo túnel (Tor ou Proxy).
  - **Todo o restante** (áudio, vídeo WebRTC, Go Live, chat, uploads) sai **direto pela sua internet local**.
- **Validação Dupla de Conexão TLS**:
  Antes de abrir o Discord, o programa testa a rota:
  1. `https://cloudflare.com/cdn-cgi/trace`: confirma túnel ativo, certificado válido e país de saída fora do Brasil (`loc!=BR`).
  2. `https://gateway.discord.gg`: confirma que os servidores do Discord estão alcançáveis pelo túnel.

## 2. Como Usar

Ao dar duplo clique no executável (ou rodar no terminal), o menu interativo é exibido:

```text
[1] Usar Rede Tor (Automático)
[2] Usar Proxy / VPN personalizada
[0] Sair

Escolha uma opção:
```

### Opções:

1. **Opção [1]**: Baixa o Tor portátil oficial automaticamente para a pasta temporária do sistema (`/tmp` ou `%TEMP%`), inicia de forma silenciosa, valida o túnel e abre o Discord.
2. **Opção [2]**: Permite digitar o endereço de uma Proxy ou VPN própria (ex: `127.0.0.1:1080`, `socks5://user:pass@host:port` ou `http://host:port`). O programa valida a rota antes de fechar o Discord anterior. Se o teste falhar, avisa o erro e não altera o Discord.
3. **Opção [0]**: Encerra o programa.

### Encerramento:

- Se você pressionar `Ctrl+C` no terminal, o Discord e o túnel são finalizados juntos.
- Se você fechar a janela do Discord, o túnel é encerrado e o terminal exibe `Pressione Enter para sair...` (evitando fechar a janela do CMD no Windows de forma repentina).

---

## 3. Como Compilar (Build)

```bash
# 1. Compilar para o sistema atual (macOS ou Linux)
make build
# -> Gera: build/discord-tor

# 2. Compilar para Windows (Cross-compilation 64 bits)
make build-windows
# -> Gera: build/discord-tor.exe

# 3. Executar os testes unitários automatizados
make test
```
