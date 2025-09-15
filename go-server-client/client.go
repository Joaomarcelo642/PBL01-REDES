// Pacote principal, o ponto de entrada da aplicação.
package main

// Importa as bibliotecas necessárias para rede, concorrência, I/O e manipulação de dados.
import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// O 'stateMutex' protege o acesso às variáveis de estado globais 'isSearching' e 'isInGame'.
// Isso é crucial para evitar que diferentes partes do programa (goroutines) as modifiquem
// ao mesmo tempo, garantindo a consistência do estado do cliente (ex: não procurar partida enquanto já está em uma).
var stateMutex sync.Mutex
var isSearching bool
var isInGame bool

// Tempo máximo, em segundos, que o cliente ficará na fila de matchmaking.
const matchmakingTimeoutSeconds = 15

// Função principal que inicializa e executa o cliente.
func main() {
	// Define e processa flags de linha de comando para permitir a execução em modo "bot",
	// útil para testes de carga e simulações com múltiplos clientes automatizados.
	botMode := flag.Bool("bot", false, "Executa o cliente em modo automatizado (bot).")
	botCount := flag.Int("count", 1, "Número de bots a serem executados em paralelo.")
	botPrefix := flag.String("prefix", "Jogador", "Prefixo para o nome dos bots.")
	flag.Parse()

	// Pega os argumentos que não são flags, como o IP do servidor.
	args := flag.Args()
	if len(args) < 1 {
		log.Fatal("Uso: ./client [-bot] [-count N] [-prefix P] <ip_do_servidor> [nome_do_jogador_manual]")
	}
	serverIP := args[0]

	// Se o modo bot estiver ativado, o programa irá simular múltiplos jogadores.
	if *botMode {
		// 'WaitGroup' é usado para garantir que a função 'main' espere todos os bots terminarem.
		var wg sync.WaitGroup
		for i := 1; i <= *botCount; i++ {
			wg.Add(1) // Incrementa o contador para cada bot iniciado.
			playerName := fmt.Sprintf("%s%d", *botPrefix, i)
			serverTcpAddr := fmt.Sprintf("%s:8080", serverIP)
			serverUdpAddr := fmt.Sprintf("%s:8081", serverIP)
			// Um pequeno atraso para não sobrecarregar o servidor com conexões simultâneas.
			time.Sleep(10 * time.Millisecond)
			// Cada bot é executado em sua própria goroutine para rodar em paralelo.
			go func() {
				defer wg.Done() // Decrementa o contador quando o bot finaliza.
				runBot(playerName, serverTcpAddr, serverUdpAddr)
			}()
		}
		wg.Wait() // Bloqueia até que todos os bots tenham finalizado.
		log.Printf("Todos os %d bots terminaram a execução.", *botCount)
	} else {
		// Modo interativo para um jogador humano.
		if len(args) < 2 {
			log.Fatal("Uso para modo interativo: ./client <ip_do_servidor> <nome_do_jogador>")
		}
		playerName := args[1]
		serverTcpAddr := fmt.Sprintf("%s:8080", serverIP)
		serverUdpAddr := fmt.Sprintf("%s:8081", serverIP)
		// Inicia o envio de pacotes UDP (keep-alive) em segundo plano.
		go sendUdpData(playerName, serverUdpAddr)
		// Gerencia a conexão TCP principal e a interação com o usuário.
		handleServerConnection(playerName, serverTcpAddr)
	}
}

// 'runBot' define o comportamento de um cliente automatizado.
func runBot(playerName string, serverTcpAddr string, serverUdpAddr string) {
	go sendUdpData(playerName, serverUdpAddr)

	conn, err := net.Dial("tcp", serverTcpAddr)
	if err != nil {
		log.Printf("[Bot %s]: Não foi possível conectar ao servidor: %v", playerName, err)
		return
	}
	defer conn.Close()

	fmt.Fprintf(conn, "%s\n", playerName)

	reader := bufio.NewReader(conn)
	// Espera a resposta inicial do servidor para confirmar a conexão.
	_, errReadInitial := reader.ReadString('\n')
	if errReadInitial != nil {
		if errReadInitial != io.EOF {
			log.Printf("[Bot %s]: Erro ao receber pacote inicial: %v", playerName, errReadInitial)
		}
		return
	}
	log.Printf("[Bot %s]: Pacote inicial recebido.", playerName)

	// Ação automatizada: O bot abre 2 pacotes de cartas.
	log.Printf("[Bot %s]: Abrindo 2 pacotes extras...", playerName)
	for i := 0; i < 2; i++ {
		fmt.Fprintf(conn, "OPEN_PACK\n")
		_, errReadPack := reader.ReadString('\n')
		if errReadPack != nil {
			break
		}
		log.Printf("[Bot %s]: Pacote extra %d aberto.", playerName, i+1)
	}

	// Ação automatizada: O bot entra na fila para uma partida.
	log.Printf("[Bot %s]: Procurando partida...", playerName)
	fmt.Fprintf(conn, "FIND_MATCH\n")

	// Loop principal do bot, que reage às mensagens do servidor.
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			break // Sai do loop se a conexão for perdida.
		}

		message = strings.TrimSpace(message)

		if strings.HasPrefix(message, "MATCH_START|") {
			// Ao iniciar a partida, o bot joga a primeira carta ("1") automaticamente.
			log.Printf("[Bot %s]: Partida iniciada! Jogando...", playerName)
			fmt.Fprintf(conn, "1\n")
		} else if strings.HasPrefix(message, "RESULT|") {
			// Ao receber o resultado, o bot encerra sua execução.
			log.Printf("[Bot %s]: Partida finalizada.", playerName)
			break
		} else if message == "NO_MATCH_FOUND" {
			log.Printf("[Bot %s]: Nenhum oponente encontrado. Encerrando.", playerName)
			break
		}
	}
	log.Printf("[Bot %s]: Desconectando.", playerName)
}

// 'handleServerConnection' gerencia a lógica para um jogador humano.
func handleServerConnection(playerName string, serverTcpAddr string) {
	var conn net.Conn
	var err error

	// Tenta se conectar ao servidor com um número máximo de retentativas.
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		conn, err = net.Dial("tcp", serverTcpAddr)
		if err == nil {
			break // Conexão bem-sucedida.
		}
		log.Printf("%s: Falha ao conectar ao servidor (%v). Tentando novamente em 2 segundos...", playerName, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("%s: Não foi possível conectar ao servidor após %d tentativas.", playerName, maxRetries)
	}
	defer conn.Close()

	fmt.Fprintf(conn, "%s\n", playerName)
	log.Printf("%s: Conectado com sucesso!", playerName)

	_, cancelGame := context.WithCancel(context.Background())
	defer cancelGame()

	// Inicia uma goroutine para ouvir mensagens do servidor de forma assíncrona,
	// para que o programa não bloqueie esperando por elas.
	go listenServerMessages(conn, playerName, cancelGame)

	// Loop principal que lê a entrada do teclado do usuário.
	reader := bufio.NewReader(os.Stdin)
	for {
		// A verificação do estado com mutex garante que o menu só seja exibido
		// quando o jogador não estiver em uma ação que bloqueie o menu (como buscar partida ou jogar).
		stateMutex.Lock()
		canShowMenu := !isSearching && !isInGame
		stateMutex.Unlock()

		if canShowMenu {
			showMenu()
			input, _ := reader.ReadString('\n')
			choice := strings.TrimSpace(input)

			// Envia comandos para o servidor com base na escolha do usuário.
			switch choice {
			case "1":
				stateMutex.Lock()
				isSearching = true // Atualiza o estado para "procurando".
				stateMutex.Unlock()
				fmt.Fprintf(conn, "FIND_MATCH\n")
				go runSearchCountdown(matchmakingTimeoutSeconds) // Inicia o contador visual.
			case "2":
				fmt.Fprintf(conn, "OPEN_PACK\n")
			case "3":
				fmt.Fprintf(conn, "VIEW_DECK\n")
			case "4":
				return // Encerra a função e o programa.
			default:
				fmt.Println("Opção inválida. Tente novamente.")
			}
		}
		time.Sleep(100 * time.Millisecond) // Pausa para evitar uso excessivo de CPU.
	}
}

// 'showMenu' apenas exibe as opções de ação para o jogador.
func showMenu() {
	fmt.Println("\n--- MENU PRINCIPAL ---")
	fmt.Println("1. Procurar Partida")
	fmt.Println("2. Abrir Pacote de Cartas")
	fmt.Println("3. Ver Meu Deck")
	fmt.Println("4. Sair")
	fmt.Print("> ")
}

// 'listenServerMessages' roda em background para processar todas as mensagens recebidas do servidor.
func listenServerMessages(conn net.Conn, playerName string, cancelGame context.CancelFunc) {
	reader := bufio.NewReader(conn)
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			// Se a conexão for fechada pelo servidor (EOF) ou perdida, encerra o cliente.
			if err == io.EOF {
				log.Printf("%s: O servidor fechou a conexão.", playerName)
			} else {
				log.Printf("%s: Conexão com o servidor perdida: %v", playerName, err)
			}
			os.Exit(0)
		}

		message = strings.TrimSpace(message)
		fmt.Printf("\r%s\n", strings.Repeat(" ", 50)) // Limpa a linha atual antes de exibir a mensagem.

		// Trata as diferentes mensagens do servidor, atualizando o estado do cliente conforme necessário.
		if strings.HasPrefix(message, "MATCH_START|") {
			stateMutex.Lock()
			isSearching = false
			isInGame = true
			stateMutex.Unlock()
			gameCtx, newCancel := context.WithCancel(context.Background())
			cancelGame = newCancel
			handleGame(gameCtx, conn, message)
		} else if strings.HasPrefix(message, "RESULT|") {
			cancelGame() // Cancela a leitura de jogada, se estiver pendente.
			parts := strings.SplitN(message, "|", 2)
			fmt.Printf("\r--- FIM DA PARTIDA ---\n%s\n---------------------\n", parts[1])
			stateMutex.Lock()
			isInGame = false // Retorna ao estado ocioso.
			stateMutex.Unlock()
		} else if message == "MATCH_FOUND" {
			fmt.Printf("\r[Servidor]: Partida encontrada! Iniciando...\n")
			stateMutex.Lock()
			isSearching = false
			stateMutex.Unlock()
		} else if message == "NO_MATCH_FOUND" {
			fmt.Printf("\r[Servidor]: Nenhum oponente encontrado a tempo. Tente novamente.\n")
			stateMutex.Lock()
			isSearching = false // Retorna ao estado ocioso.
			stateMutex.Unlock()
		} else if strings.HasPrefix(message, "TIMER|") {
			parts := strings.Split(message, "|")
			seconds, _ := strconv.Atoi(parts[1])
			go runGameCountdown(seconds) // Inicia o contador de tempo de jogada.
		} else {
			// Exibe qualquer outra mensagem genérica do servidor.
			fmt.Printf("\r[Servidor]: %s\n", message)
		}

		// Se o jogador não estiver ocupado, reexibe o prompt ">" para a próxima ação.
		stateMutex.Lock()
		if !isSearching && !isInGame {
			fmt.Print("> ")
		}
		stateMutex.Unlock()
	}
}

// 'handleGame' exibe a mão do jogador e inicia a captura da sua jogada.
func handleGame(ctx context.Context, conn net.Conn, message string) {
	parts := strings.Split(message, "|")
	card1 := parts[1]
	card2 := parts[2]

	fmt.Println("\r--- PARTIDA INICIADA ---")
	fmt.Println("Sua mão:")
	fmt.Printf("1: %s\n", card1)
	fmt.Printf("2: %s\n", card2)
	fmt.Print("Escolha sua carta (1 ou 2): > ")

	// Inicia a leitura da jogada em uma goroutine para não bloquear o programa.
	go readPlayerInput(ctx, conn)
}

// 'readPlayerInput' gerencia a entrada do jogador durante uma partida.
func readPlayerInput(ctx context.Context, conn net.Conn) {
	choiceChan := make(chan string)
	reader := bufio.NewReader(os.Stdin)

	// Lê a entrada do teclado em uma goroutine separada para não travar.
	go func() {
		input, err := reader.ReadString('\n')
		if err == nil {
			choiceChan <- strings.TrimSpace(input)
		}
	}()

	// O 'select' aguarda por dois eventos simultaneamente:
	// 1. A jogada do usuário chegar pelo canal 'choiceChan'.
	// 2. O contexto ser cancelado ('ctx.Done()'), o que acontece se o jogo terminar
	//    antes que o jogador faça sua jogada (ex: tempo esgotado).
	select {
	case choice := <-choiceChan:
		fmt.Fprintf(conn, "%s\n", choice)
		fmt.Println("Jogada enviada. Aguardando resultado...")
	case <-ctx.Done():
		fmt.Println("\nA partida terminou antes de você fazer uma jogada.")
		return
	}
}

// 'runSearchCountdown' mostra um contador visual enquanto procura uma partida.
func runSearchCountdown(seconds int) {
	for i := seconds; i > 0; i-- {
		stateMutex.Lock()
		// Se a busca for cancelada (ex: partida encontrada), o contador para.
		if !isSearching {
			stateMutex.Unlock()
			fmt.Printf("\r%s\r", strings.Repeat(" ", 50)) // Limpa a linha.
			return
		}
		stateMutex.Unlock()

		fmt.Printf("\rBuscando partida... Tempo restante: %d segundos ", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Printf("\r%s\r", strings.Repeat(" ", 50))
}

// 'runGameCountdown' mostra um contador visual para o tempo de jogada.
func runGameCountdown(seconds int) {
	for i := seconds; i > 0; i-- {
		stateMutex.Lock()
		// Se o jogo terminar, o contador para.
		if !isInGame {
			stateMutex.Unlock()
			fmt.Printf("\r%s\r", strings.Repeat(" ", 50)) // Limpa a linha.
			return
		}
		stateMutex.Unlock()

		fmt.Printf("\rTempo de jogada restante: %d segundos... ", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Printf("\r%s\r", strings.Repeat(" ", 50))
}

// 'sendUdpData' envia pacotes UDP periodicamente para o servidor.
// Funciona como um "heartbeat", permitindo ao servidor saber que o cliente ainda está ativo.
func sendUdpData(playerName string, serverUdpAddr string) {
	var conn net.Conn
	var err error

	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		udpAddr, errResolve := net.ResolveUDPAddr("udp", serverUdpAddr)
		if errResolve != nil {
			log.Printf("%s: Não foi possível resolver endereço UDP. Tentando de novo...", playerName)
			time.Sleep(2 * time.Second)
			continue
		}
		conn, err = net.DialUDP("udp", nil, udpAddr)
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Printf("%s: Não foi possível conectar ao servidor UDP após %d tentativas.", playerName, maxRetries)
		return
	}
	defer conn.Close()

	// Loop infinito que envia um pacote com nome e timestamp a cada 2 segundos.
	for {
		timestamp := time.Now().UnixNano()
		message := fmt.Sprintf("%s:%d", playerName, timestamp)
		_, errWrite := conn.Write([]byte(message))
		if errWrite != nil {
			log.Printf("%s: Erro ao enviar pacote UDP: %v", playerName, errWrite)
		}
		time.Sleep(2 * time.Second)
	}
}
