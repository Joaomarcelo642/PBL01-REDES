// Pacote principal da aplicação do servidor.
package main

// Importa as bibliotecas necessárias para o funcionamento do servidor,
// incluindo rede (net), concorrência (sync, atomic), I/O (bufio),
// e captura de sinais do sistema operacional (os/signal).
import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Constantes globais que definem configurações importantes do servidor,
// como as portas de comunicação e os tempos de espera (timeouts).
const (
	tcpPort            = ":8080"
	udpPort            = ":8081"
	matchmakingTimeout = 15 * time.Second
	gameTurnTimeout    = 10 * time.Second
)

// --- ESTRUTURAS DE DADOS ---

// 'Card' representa uma única carta do jogo, com nome e força.
type Card struct {
	Name  string
	Forca int
}

// 'Player' representa um jogador conectado, armazenando seu nome, conexão de rede,
// as cartas que possui (Deck) e quantos pacotes já abriu.
type Player struct {
	Name        string
	Conn        net.Conn
	Deck        []Card
	PacksOpened int
}

// 'GameSession' armazena o estado de uma partida 1v1 em andamento,
// incluindo os dois jogadores e as cartas que eles jogaram.
type GameSession struct {
	Player1     *Player
	Player2     *Player
	Player1Card *Card
	Player2Card *Card
	mu          sync.Mutex // Mutex para proteger o acesso concorrente aos dados da sessão.
}

// --- VARIÁVEIS GLOBAIS ---
var (
	// 'matchmakingQueue' é um canal que funciona como uma fila para encontrar partidas.
	// Jogadores que querem jogar são enviados para este canal.
	matchmakingQueue = make(chan *Player)

	// 'cardPacks' é uma "pilha" global de pacotes de cartas disponíveis no servidor.
	cardPacks [][]Card

	// 'players' é um mapa que armazena os dados de todos os jogadores atualmente conectados.
	players = make(map[net.Conn]*Player)

	// Mutexes para garantir que o acesso a recursos compartilhados (estoque de cartas e
	// lista de jogadores) seja seguro em um ambiente com múltiplas goroutines.
	stockMutex  = &sync.Mutex{}
	playerMutex = &sync.Mutex{}

	// Contadores atômicos para coletar estatísticas de uso do servidor de forma segura.
	totalConnections   int64
	totalPacksOpened   int64
	totalMatchesPlayed int64
)

// Função principal que inicializa e orquestra o servidor.
func main() {
	// Prepara o estoque de cartas e os servidores TCP e UDP.
	initializeCardPacks()
	go startTcpServer()
	go matchmaker()
	go startUdpServer()

	fmt.Println("Servidor iniciado. Pressione Ctrl+C para encerrar e ver as estatísticas.")

	// Este bloco captura o sinal de interrupção (Ctrl+C) para permitir um encerramento
	// "gracioso", exibindo as estatísticas coletadas antes de o programa fechar.
	quitChannel := make(chan os.Signal, 1)
	signal.Notify(quitChannel, syscall.SIGINT, syscall.SIGTERM)
	<-quitChannel // Bloqueia a execução até que o sinal seja recebido.

	fmt.Println("\n--- ESTATÍSTICAS DO TESTE DE ESTRESSE ---")
	fmt.Printf("Total de conexões TCP estabelecidas: %d\n", atomic.LoadInt64(&totalConnections))
	fmt.Printf("Total de pacotes de cartas abertos: %d\n", atomic.LoadInt64(&totalPacksOpened))
	fmt.Printf("Total de partidas 1v1 realizadas: %d\n", atomic.LoadInt64(&totalMatchesPlayed))
	fmt.Println("-------------------------------------------")
}

// 'initializeCardPacks' cria o "baralho" completo do servidor, com múltiplas cópias
// de cada carta baseadas em sua raridade/força, embaralha tudo e divide em pacotes de 3 cartas.
func initializeCardPacks() {
	// Definição de todas as cartas possíveis no jogo.
	baseCards := []Card{
		{Name: "Camponês Armado", Forca: 1}, {Name: "Batedor Anão", Forca: 1}, {Name: "Arqueiro Elfo", Forca: 1},
		{Name: "Ghoul", Forca: 1}, {Name: "Nekker", Forca: 1}, {Name: "Infantaria Leve", Forca: 2},
		{Name: "Guerrilheiro Scoia'tael", Forca: 2}, {Name: "Balista", Forca: 2}, {Name: "Lanceiro de Kaedwen", Forca: 3},
		{Name: "Caçador de Recompensa", Forca: 3}, {Name: "Grifo", Forca: 3}, {Name: "Cavaleiro de Aedirn", Forca: 4},
		{Name: "Elemental da Terra", Forca: 4}, {Name: "Guerreiro Anão", Forca: 5}, {Name: "Wyvern", Forca: 5},
		{Name: "Gigante de Gelo", Forca: 6}, {Name: "Leshen", Forca: 6}, {Name: "Grão-Mestre Bruxo", Forca: 7},
		{Name: "Draug", Forca: 7}, {Name: "Ifrit", Forca: 8}, {Name: "Cavaleiro da Morte", Forca: 8},
		{Name: "Behemoth", Forca: 9}, {Name: "Dragão Menor", Forca: 10}, {Name: "Comandante Veterano", Forca: 10},
		{Name: "Eredin Bréacc Glas", Forca: 11}, {Name: "Imlerith", Forca: 11}, {Name: "Vernon Roche", Forca: 12},
		{Name: "Iorveth", Forca: 12}, {Name: "Philippa Eilhart", Forca: 13}, {Name: "Triss Merigold", Forca: 13},
		{Name: "Yennefer de Vengerberg", Forca: 14}, {Name: "Rei Foltest", Forca: 14}, {Name: "Geralt de Rívia", Forca: 15},
	}

	// Cria um grande estoque de cartas, com mais cópias das cartas mais fracas.
	fullCardStock := []Card{}
	for _, card := range baseCards {
		copies := 1
		if card.Forca >= 1 && card.Forca <= 3 {
			copies = 400
		} else if card.Forca >= 4 && card.Forca <= 6 {
			copies = 300
		} else if card.Forca >= 7 && card.Forca <= 10 {
			copies = 200
		}
		for i := 0; i < copies; i++ {
			fullCardStock = append(fullCardStock, card)
		}
	}

	// Garante que o estoque tenha exatamente 9000 cartas para formar 3000 pacotes.
	for len(fullCardStock) < 9000 {
		fullCardStock = append(fullCardStock, baseCards[0])
	}
	fullCardStock = fullCardStock[:9000]

	// Embaralha o estoque para garantir que os pacotes sejam aleatórios.
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(fullCardStock), func(i, j int) {
		fullCardStock[i], fullCardStock[j] = fullCardStock[j], fullCardStock[i]
	})

	// Divide o estoque em pacotes de 3 cartas.
	for i := 0; i < 3000; i++ {
		pack := fullCardStock[i*3 : (i+1)*3]
		cardPacks = append(cardPacks, pack)
	}
	log.Printf("Estoque de cartas e pacotes inicializado. Total de pacotes: %d", len(cardPacks))
}

// 'startTcpServer' abre a porta TCP e fica em um loop infinito, aceitando novas conexões de jogadores.
// Cada nova conexão é tratada em sua própria goroutine para não bloquear o servidor.
func startTcpServer() {
	listener, err := net.Listen("tcp", tcpPort)
	if err != nil {
		log.Fatalf("Erro ao iniciar o servidor TCP: %v", err)
	}
	defer listener.Close()
	log.Println("Servidor TCP iniciado na porta", tcpPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Erro ao aceitar conexão TCP: %v", err)
			continue
		}
		// A função 'handlePlayerConnection' cuidará de toda a comunicação com este novo jogador.
		go handlePlayerConnection(conn)
	}
}

// 'handlePlayerConnection' gerencia o ciclo de vida de um jogador conectado,
// desde o registro inicial até o processamento de comandos.
func handlePlayerConnection(conn net.Conn) {
	atomic.AddInt64(&totalConnections, 1) // Incrementa o contador de conexões.

	// Verifica se o jogador já existe; se não, cria um novo.
	playerMutex.Lock()
	player, playerExists := players[conn]
	playerMutex.Unlock()

	if !playerExists {
		// Lê o nome do jogador da primeira mensagem enviada.
		reader := bufio.NewReader(conn)
		name, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Erro ao ler o nome: %v", err)
			conn.Close()
			return
		}
		name = strings.TrimSpace(name)
		player = &Player{Name: name, Conn: conn, Deck: []Card{}, PacksOpened: 0}

		// Adiciona o novo jogador ao mapa de jogadores ativos.
		playerMutex.Lock()
		players[conn] = player
		playerMutex.Unlock()
		log.Printf("Jogador %s conectado de %s", player.Name, player.Conn.RemoteAddr())

		// Todo jogador novo recebe um pacote de cartas inicial.
		openCardPack(player, true)
	}

	// Loop principal que lê e processa os comandos enviados pelo jogador.
	reader := bufio.NewReader(conn)
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			// Se houver um erro de leitura (ex: desconexão), remove o jogador e encerra.
			log.Printf("Jogador %s desconectado.", player.Name)
			playerMutex.Lock()
			delete(players, conn)
			playerMutex.Unlock()
			conn.Close()
			return
		}

		command := strings.TrimSpace(message)
		log.Printf("Comando recebido de %s: %s", player.Name, command)

		// Processa o comando recebido.
		switch command {
		case "FIND_MATCH":
			// Este loop com timeout baixo serve para "limpar" o buffer de rede, processando
			// comandos 'OPEN_PACK' que possam ter sido enviados em rápida sucessão pelo cliente
			// antes de entrar na fila, evitando que o comando se perca.
			for {
				player.Conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
				message, err := reader.ReadString('\n')
				player.Conn.SetReadDeadline(time.Time{})
				if err != nil {
					break // Sai do loop se não houver mais dados no buffer.
				}
				extraCommand := strings.TrimSpace(message)
				if extraCommand == "OPEN_PACK" {
					log.Printf("Processando 'OPEN_PACK' em buffer para %s", player.Name)
					openCardPack(player, false)
				}
			}

			// Envia o jogador para a fila de matchmaking e encerra esta função,
			// pois o controle da conexão será do 'matchmaker' ou da 'GameSession'.
			matchmakingQueue <- player
			return
		case "OPEN_PACK":
			openCardPack(player, false)
		case "VIEW_DECK":
			viewDeck(player)
		default:
			player.Conn.Write([]byte("Comando inválido.\n"))
		}
	}
}

// 'openCardPack' remove um pacote do estoque global e o adiciona ao deck de um jogador.
// O uso de 'stockMutex' garante que a operação no estoque de pacotes seja segura.
func openCardPack(player *Player, isMandatory bool) {
	stockMutex.Lock()
	defer stockMutex.Unlock()

	if player.PacksOpened >= 3 {
		player.Conn.Write([]byte("Você já abriu o máximo de 3 pacotes.\n"))
		return
	}

	if len(cardPacks) > 0 {
		pack := cardPacks[0]
		cardPacks = cardPacks[1:] // Remove o pacote do topo da "pilha".
		player.Deck = append(player.Deck, pack...)
		player.PacksOpened++
		atomic.AddInt64(&totalPacksOpened, 1) // Atualiza as estatísticas.

		// Constrói e envia uma resposta ao jogador com as cartas recebidas.
		var response string
		if isMandatory {
			response = "Bem-vindo! Você recebeu seu pacote inicial: "
		} else {
			response = fmt.Sprintf("Parabéns, %s! Você abriu um pacote e recebeu: ", player.Name)
		}
		for i, card := range pack {
			response += fmt.Sprintf("%s (Força: %d)", card.Name, card.Forca)
			if i < len(pack)-1 {
				response += ", "
			}
		}
		response += fmt.Sprintf(". Pacotes restantes no servidor: %d\n", len(cardPacks))
		player.Conn.Write([]byte(response))
	} else {
		player.Conn.Write([]byte("Desculpe, não há mais pacotes de cartas disponíveis no servidor.\n"))
	}
}

// 'viewDeck' envia ao jogador uma lista de todas as cartas em seu deck.
func viewDeck(player *Player) {
	if len(player.Deck) == 0 {
		player.Conn.Write([]byte("Seu deck está vazio.\n"))
		return
	}
	response := "Seu deck: "
	for i, card := range player.Deck {
		response += fmt.Sprintf("%s (Força: %d)", card.Name, card.Forca)
		if i < len(player.Deck)-1 {
			response += " | "
		}
	}
	player.Conn.Write([]byte(response + "\n"))
}

// 'matchmaker' é uma goroutine que gerencia a fila de espera por partidas.
// Ele pareia jogadores ou remove um jogador da fila se o tempo de espera esgotar.
func matchmaker() {
	var waitingPlayer *Player
	var timer *time.Timer
	var timeoutChan <-chan time.Time

	for {
		// O 'select' aguarda por um de dois eventos: um novo jogador entrando na fila,
		// ou o timer do jogador que já está esperando estourar.
		select {
		case newPlayer := <-matchmakingQueue:
			log.Printf("Jogador %s entrou na fila...", newPlayer.Name)
			if waitingPlayer == nil {
				// Se não há ninguém esperando, este novo jogador se torna o 'waitingPlayer'.
				waitingPlayer = newPlayer
				timer = time.NewTimer(matchmakingTimeout)
				timeoutChan = timer.C
				log.Printf("Jogador %s está aguardando. Timeout de %v iniciado.", waitingPlayer.Name, matchmakingTimeout)
			} else {
				// Se já havia um jogador esperando, uma partida é formada.
				timer.Stop()
				log.Printf("Partida encontrada: %s vs %s", waitingPlayer.Name, newPlayer.Name)
				waitingPlayer.Conn.Write([]byte("MATCH_FOUND\n"))
				newPlayer.Conn.Write([]byte("MATCH_FOUND\n"))
				// A partida é iniciada em uma nova goroutine.
				go handleGameSession(waitingPlayer, newPlayer)
				waitingPlayer = nil // Limpa a fila para o próximo par.
				timeoutChan = nil
			}
		case <-timeoutChan:
			// Se o tempo de espera esgotar, o jogador é removido da fila.
			if waitingPlayer != nil {
				log.Printf("Timeout para %s. Nenhuma partida encontrada.", waitingPlayer.Name)
				waitingPlayer.Conn.Write([]byte("NO_MATCH_FOUND\n"))
				// O jogador é devolvido para o loop de tratamento de conexão normal.
				go handlePlayerConnection(waitingPlayer.Conn)
				waitingPlayer = nil
				timeoutChan = nil
			}
		}
	}
}

// 'handleGameSession' gerencia uma partida 1v1 do início ao fim.
func handleGameSession(player1 *Player, player2 *Player) {
	atomic.AddInt64(&totalMatchesPlayed, 1)

	// O 'defer' garante que, ao final da partida, ambos os jogadores
	// retornem ao loop 'handlePlayerConnection' para que possam jogar novamente ou sair.
	defer func() {
		go handlePlayerConnection(player1.Conn)
		go handlePlayerConnection(player2.Conn)
	}()

	log.Printf("Iniciando sessão de jogo entre %s e %s.", player1.Name, player2.Name)

	// Seleciona 2 cartas aleatórias do deck de cada jogador para formar a "mão".
	hand1 := selectRandomCards(player1.Deck, 2)
	hand2 := selectRandomCards(player2.Deck, 2)
	if hand1 == nil || hand2 == nil {
		msg := "Erro: Cartas insuficientes no deck para iniciar a partida.\n"
		player1.Conn.Write([]byte(msg))
		player2.Conn.Write([]byte(msg))
		return
	}

	// Envia a "mão" para cada jogador e informa o tempo do turno.
	p1HandStr := fmt.Sprintf("MATCH_START|%s (%d)|%s (%d)\n", hand1[0].Name, hand1[0].Forca, hand1[1].Name, hand1[1].Forca)
	p2HandStr := fmt.Sprintf("MATCH_START|%s (%d)|%s (%d)\n", hand2[0].Name, hand2[0].Forca, hand2[1].Name, hand2[1].Forca)
	player1.Conn.Write([]byte(p1HandStr))
	player2.Conn.Write([]byte(p2HandStr))
	timerMsg := fmt.Sprintf("TIMER|%d\n", int(gameTurnTimeout.Seconds()))
	player1.Conn.Write([]byte(timerMsg))
	player2.Conn.Write([]byte(timerMsg))

	session := &GameSession{Player1: player1, Player2: player2}
	cardPlayed := make(chan struct{}, 2) // Canal para sinalizar quando um jogador joga uma carta.

	// Inicia goroutines para ouvir a jogada de cada jogador.
	go listenForPlayerMove(session, player1, hand1, cardPlayed)
	go listenForPlayerMove(session, player2, hand2, cardPlayed)

	// Este loop aguarda até que ambos os jogadores tenham feito sua jogada
	// ou até que o tempo do turno se esgote.
	timeout := time.After(gameTurnTimeout)
	moves := 0
Loop:
	for {
		select {
		case <-cardPlayed:
			moves++
			if moves == 2 {
				break Loop // Sai se ambos jogaram.
			}
		case <-timeout:
			log.Printf("Timeout da partida entre %s e %s.", player1.Name, player2.Name)
			break Loop // Sai se o tempo acabou.
		}
	}

	// Após o fim do turno, determina e anuncia o vencedor.
	determineWinner(session)
}

// 'selectRandomCards' pega uma cópia do deck do jogador, embaralha e retorna o número de cartas solicitado.
func selectRandomCards(deck []Card, count int) []Card {
	if len(deck) < count {
		return nil
	}
	rand.Seed(time.Now().UnixNano())
	deckCopy := make([]Card, len(deck))
	copy(deckCopy, deck)
	rand.Shuffle(len(deckCopy), func(i, j int) {
		deckCopy[i], deckCopy[j] = deckCopy[j], deckCopy[i]
	})
	return deckCopy[:count]
}

// 'listenForPlayerMove' aguarda a jogada de um único jogador.
// Define um 'deadline' na conexão para não ficar esperando para sempre.
func listenForPlayerMove(session *GameSession, player *Player, hand []Card, cardPlayed chan struct{}) {
	player.Conn.SetReadDeadline(time.Now().Add(gameTurnTimeout + 2*time.Second))
	reader := bufio.NewReader(player.Conn)
	message, err := reader.ReadString('\n')
	player.Conn.SetReadDeadline(time.Time{}) // Remove o deadline após a leitura.

	if err != nil {
		log.Printf("Jogador %s não fez uma jogada ou desconectou: %v", player.Name, err)
		return
	}

	// Converte a escolha do jogador (string "1" ou "2") para um índice.
	choiceStr := strings.TrimSpace(message)
	choice, err := strconv.Atoi(choiceStr)
	if err != nil || (choice != 1 && choice != 2) {
		return
	}

	playedCard := hand[choice-1]

	// Armazena a carta jogada na sessão de jogo de forma segura.
	session.mu.Lock()
	if player.Name == session.Player1.Name {
		session.Player1Card = &playedCard
	} else {
		session.Player2Card = &playedCard
	}
	session.mu.Unlock()

	log.Printf("%s jogou a carta: %s", player.Name, playedCard.Name)
	cardPlayed <- struct{}{} // Sinaliza que a jogada foi feita.
}

// 'determineWinner' compara as cartas jogadas e envia o resultado para ambos os jogadores.
// Também trata os casos em que um ou ambos os jogadores não jogaram a tempo (timeout).
func determineWinner(session *GameSession) {
	session.mu.Lock()
	defer session.mu.Unlock()

	p1Card := session.Player1Card
	p2Card := session.Player2Card
	var resultP1, resultP2, logMessage string

	if p1Card != nil && p2Card != nil {
		// Caso 1: Ambos jogaram. Compara a força das cartas.
		if p1Card.Forca > p2Card.Forca {
			resultP1 = fmt.Sprintf("RESULT|VITÓRIA|Sua carta %s (%d) venceu %s (%d) de %s.\n", p1Card.Name, p1Card.Forca, p2Card.Name, p2Card.Forca, session.Player2.Name)
			resultP2 = fmt.Sprintf("RESULT|DERROTA|Sua carta %s (%d) perdeu para %s (%d) de %s.\n", p2Card.Name, p2Card.Forca, p1Card.Name, p1Card.Forca, session.Player1.Name)
			logMessage = fmt.Sprintf("Resultado: %s venceu %s.", session.Player1.Name, session.Player2.Name)
		} else if p2Card.Forca > p1Card.Forca {
			resultP2 = fmt.Sprintf("RESULT|VITÓRIA|Sua carta %s (%d) venceu %s (%d) de %s.\n", p2Card.Name, p2Card.Forca, p1Card.Name, p1Card.Forca, session.Player1.Name)
			resultP1 = fmt.Sprintf("RESULT|DERROTA|Sua carta %s (%d) perdeu para %s (%d) de %s.\n", p1Card.Name, p1Card.Forca, p2Card.Name, p2Card.Forca, session.Player2.Name)
			logMessage = fmt.Sprintf("Resultado: %s venceu %s.", session.Player2.Name, session.Player1.Name)
		} else {
			result := fmt.Sprintf("RESULT|EMPATE|Empate! Ambas as cartas têm força %d.\n", p1Card.Forca)
			resultP1, resultP2 = result, result
			logMessage = fmt.Sprintf("Resultado: Empate entre %s e %s.", session.Player1.Name, session.Player2.Name)
		}
	} else if p1Card == nil && p2Card != nil {
		// Caso 2: Apenas o jogador 2 jogou.
		resultP1 = "RESULT|DERROTA|Você não jogou a tempo e perdeu.\n"
		resultP2 = fmt.Sprintf("RESULT|VITÓRIA|%s não jogou a tempo. Você venceu!\n", session.Player1.Name)
		logMessage = fmt.Sprintf("Resultado: %s venceu %s por timeout.", session.Player2.Name, session.Player1.Name)
	} else if p2Card == nil && p1Card != nil {
		// Caso 3: Apenas o jogador 1 jogou.
		resultP2 = "RESULT|DERROTA|Você não jogou a tempo e perdeu.\n"
		resultP1 = fmt.Sprintf("RESULT|VITÓRIA|%s não jogou a tempo. Você venceu!\n", session.Player2.Name)
		logMessage = fmt.Sprintf("Resultado: %s venceu %s por timeout.", session.Player1.Name, session.Player2.Name)
	} else {
		// Caso 4: Nenhum jogador jogou.
		result := "RESULT|EMPATE|Nenhum jogador jogou a tempo. Empate.\n"
		resultP1, resultP2 = result, result
		logMessage = fmt.Sprintf("Resultado: Empate por timeout duplo entre %s e %s.", session.Player1.Name, session.Player2.Name)
	}

	session.Player1.Conn.Write([]byte(resultP1))
	session.Player2.Conn.Write([]byte(resultP2))
	log.Printf("Partida entre %s e %s finalizada. %s", session.Player1.Name, session.Player2.Name, logMessage)
}

// 'startUdpServer' inicia um servidor UDP simples. Sua principal função é receber pacotes
// dos clientes e calcular a latência (ping), imprimindo-a no console do servidor.
// É útil para monitorar a qualidade da conexão dos clientes.
func startUdpServer() {
	udpAddr, err := net.ResolveUDPAddr("udp", udpPort)
	if err != nil {
		log.Fatalf("Erro ao resolver endereço UDP: %v", err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("Erro ao iniciar o servidor UDP: %v", err)
	}
	defer conn.Close()
	log.Println("Servidor UDP de tempo real iniciado na porta", udpPort)
	buffer := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Erro ao ler pacote UDP: %v", err)
			continue
		}
		// Extrai o nome do jogador e o timestamp de envio do pacote.
		message := string(buffer[:n])
		parts := strings.Split(message, ":")
		if len(parts) != 2 {
			continue
		}
		playerName := parts[0]
		sentTimestamp, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		// Calcula a latência em microssegundos e exibe.
		currentTimestamp := time.Now().UnixNano()
		latency := (currentTimestamp - sentTimestamp) / int64(time.Microsecond)
		fmt.Printf("Pacote UDP de %s recebido (%d µs de latência)\n", playerName, latency)
	}
}
