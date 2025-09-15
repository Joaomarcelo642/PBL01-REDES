# PBL01-REDES

Este projeto implementa um sistema cliente-servidor para um jogo de cartas online, desenvolvido em Go. Ele é projetado para suportar múltiplos jogadores simultaneamente, oferecendo modos de teste automatizado (estresse) e manual. O sistema utiliza comunicação **TCP** para as ações principais do jogo e **UDP** para monitoramento de latência.

---

## 🏗️ Arquitetura

O projeto é composto pelos seguintes arquivos:

* **`server.go`**: O servidor principal que gerencia a lógica do jogo, a conexão dos jogadores, o pareamento de partidas e o estoque de cartas.
* **`client.go`**: O cliente que permite a um usuário (ou bot) se conectar ao servidor para jogar.
* **`docker-compose.yml`**: Orquestra a execução do servidor e dos clientes (bots) em contêineres Docker para facilitar os testes.
* **`Dockerfile.server`** e **`Dockerfile.client`**: Definem as imagens Docker para o servidor e o cliente, respectivamente.

---

## 🚀 Como Utilizar

Para executar o projeto, você precisa ter o **Docker** e o **Docker Compose** instalados.

### 🤖 Teste Automatizado (Teste de Estresse)

Este modo simula **1000 bots** se conectando ao servidor, abrindo pacotes de cartas e procurando partidas simultaneamente. É ideal para verificar a performance e a estabilidade do servidor sob carga.

1.  **Construir e iniciar os contêineres:**
    Este comando irá construir as imagens do servidor e do cliente e iniciar os serviços em segundo plano (`-d`).
    ```bash
    docker-compose up -d --build
    ```

2.  **(Opcional) Acompanhar os logs em tempo real:**
    Para visualizar a atividade do servidor e dos bots, como conexões, abertura de pacotes e partidas sendo jogadas.
    ```bash
    docker-compose logs -f
    ```

3.  **Parar o servidor e analisar os resultados:**
    Após o teste (por exemplo, alguns minutos), pare o servidor para que ele exiba as estatísticas finais.
    ```bash
    docker-compose stop server
    ```

4.  **Visualizar as estatísticas do servidor:**
    Este comando exibe os logs do contêiner do servidor, que incluirá um resumo com o total de conexões TCP, pacotes abertos e partidas realizadas.
    ```bash
    docker-compose logs server
    ```

5.  **Encerrar e remover os contêineres:**
    Para limpar o ambiente após o teste.
    ```bash
    docker-compose down
    ```

---

### 🎮 Teste Manual

Este modo permite que você jogue manualmente, conectando-se ao servidor como um jogador único.

1.  **Iniciar apenas o servidor:**
    ```bash
    docker-compose up -d --build server
    ```

2.  **Conectar como jogador manual:**
    Abra **outro terminal** e execute o comando abaixo, substituindo `NomeDoJogador` pelo nome que desejar. Este comando executa um novo contêiner temporário (`--rm`) para o cliente e o conecta à rede do servidor.
    ```bash
    docker-compose run --rm bots ./client server NomeDoJogador
    ```
    Após a conexão, um menu interativo aparecerá, permitindo que você procure partidas, abra pacotes e veja seu deck.

3.  **Parar o servidor:**
    Quando terminar de jogar, volte ao **terminal principal** e pare o servidor.
    ```bash
    docker-compose stop server
    ```

4.  **Visualizar os logs do servidor:**
    ```bash
    docker-compose logs server
    ```

5.  **Encerrar e remover os contêineres:**
    ```bash
    docker-compose down
    ```
