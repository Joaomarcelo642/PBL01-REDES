package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"        // Adicionar
	"os/signal" // Adicionar
	"strconv"
	"strings"
	"sync"
	"sync/atomic" // Adicionar
	"syscall"     // Adicionar
	"time"
)

const (
	tcpPort            = ":8080"
	udpPort            = ":8081"
	matchmakingTimeout = 15 * time.Second
	gameTurnTimeout    = 10 * time.Second
)

// Estruturas
type Card struct {
	Name  string
	Forca int
}

type Player struct {
	Name        string
	Conn        net.Conn
	Deck        []Card
	PacksOpened int
}

type GameSession struct {
	Player1     *Player
	Player2     *Player
	Player1Card *Card
	Player2Card *Card
	mu          sync.Mutex
}

// Variáveis Globais
var (
	matchmakingQueue = make(chan *Player)
	cardPacks        [][]Card
	players          = make(map[net.Conn]*Player)
	stockMutex       = &sync.Mutex{}
	playerMutex      = &sync.Mutex{}

	totalConnections   int64
	totalPacksOpened   int64
	totalMatchesPlayed int64
)

func main() {
	initializeCardPacks()
	go startTcpServer()
	go matchmaker()
	go startUdpServer()

	fmt.Println("Servidor iniciado. Pressione Ctrl+C para encerrar e ver as estatísticas.")

	// BLOCO ADICIONADO PARA CAPTURAR SINAL E EXIBIR STATS
	quitChannel := make(chan os.Signal, 1)
	signal.Notify(quitChannel, syscall.SIGINT, syscall.SIGTERM)
	<-quitChannel

	fmt.Println("\n--- ESTATÍSTICAS DO TESTE DE ESTRESSE ---")
	fmt.Printf("Total de conexões TCP estabelecidas: %d\n", atomic.LoadInt64(&totalConnections))
	fmt.Printf("Total de pacotes de cartas abertos: %d\n", atomic.LoadInt64(&totalPacksOpened))
	fmt.Printf("Total de partidas 1v1 realizadas: %d\n", atomic.LoadInt64(&totalMatchesPlayed))
	fmt.Println("-------------------------------------------")
}

// --- Funções de Inicialização ---
func initializeCardPacks() {
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

	for len(fullCardStock) < 9000 {
		fullCardStock = append(fullCardStock, baseCards[0])
	}
	fullCardStock = fullCardStock[:9000]

	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(fullCardStock), func(i, j int) {
		fullCardStock[i], fullCardStock[j] = fullCardStock[j], fullCardStock[i]
	})

	for i := 0; i < 3000; i++ {
		pack := fullCardStock[i*3 : (i+1)*3]
		cardPacks = append(cardPacks, pack)
	}
	log.Printf("Estoque de cartas e pacotes inicializado. Total de pacotes: %d", len(cardPacks))
}

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
		go handlePlayerConnection(conn)
	}
}

func handlePlayerConnection(conn net.Conn) {
	atomic.AddInt64(&totalConnections, 1)
	playerMutex.Lock()
	player, playerExists := players[conn]
	playerMutex.Unlock()

	if !playerExists {
		reader := bufio.NewReader(conn)
		name, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Erro ao ler o nome: %v", err)
			conn.Close()
			return
		}
		name = strings.TrimSpace(name)
		player = &Player{Name: name, Conn: conn, Deck: []Card{}, PacksOpened: 0}

		playerMutex.Lock()
		players[conn] = player
		playerMutex.Unlock()
		log.Printf("Jogador %s conectado de %s", player.Name, player.Conn.RemoteAddr())
		openCardPack(player, true)
	}

	reader := bufio.NewReader(conn)
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Jogador %s desconectado.", player.Name)
			playerMutex.Lock()
			delete(players, conn)
			playerMutex.Unlock()
			conn.Close()
			return
		}

		command := strings.TrimSpace(message)
		log.Printf("Comando recebido de %s: %s", player.Name, command)

		switch command {
		case "FIND_MATCH":
			// --- INÍCIO DA CORREÇÃO DEFINITIVA ---
			// Antes de entrar na fila, drena o buffer de rede para processar comandos
			// que possam ter chegado na mesma rajada do cliente.
			for {
				// Define um timeout de leitura muito curto (ex: 20 milissegundos).
				// Se não houver nada no buffer, a leitura falhará quase instantaneamente.
				player.Conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
				message, err := reader.ReadString('\n')

				// Independentemente do resultado, limpa o timeout para futuras leituras normais.
				player.Conn.SetReadDeadline(time.Time{})

				if err != nil {
					// O erro é esperado (timeout), significa que o buffer está vazio.
					break
				}

				// Se conseguimos ler algo, processa o comando extra.
				extraCommand := strings.TrimSpace(message)
				if extraCommand == "OPEN_PACK" {
					log.Printf("Processando 'OPEN_PACK' em buffer para %s", player.Name)
					openCardPack(player, false)
				}
			}
			// --- FIM DA CORREÇÃO ---

			// Agora que o buffer está limpo, é seguro entrar na fila e encerrar o loop.
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

func openCardPack(player *Player, isMandatory bool) {
	stockMutex.Lock()
	defer stockMutex.Unlock()

	if player.PacksOpened >= 3 {
		player.Conn.Write([]byte("Você já abriu o máximo de 3 pacotes.\n"))
		return
	}

	if len(cardPacks) > 0 {
		pack := cardPacks[0]
		cardPacks = cardPacks[1:]
		player.Deck = append(player.Deck, pack...)
		player.PacksOpened++
		atomic.AddInt64(&totalPacksOpened, 1)

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

func matchmaker() {
	var waitingPlayer *Player
	var timer *time.Timer
	var timeoutChan <-chan time.Time

	for {
		select {
		case newPlayer := <-matchmakingQueue:
			log.Printf("Jogador %s entrou na fila...", newPlayer.Name)
			if waitingPlayer == nil {
				waitingPlayer = newPlayer
				timer = time.NewTimer(matchmakingTimeout)
				timeoutChan = timer.C
				log.Printf("Jogador %s está aguardando. Timeout de %v iniciado.", waitingPlayer.Name, matchmakingTimeout)
			} else {
				timer.Stop()
				log.Printf("Partida encontrada: %s vs %s", waitingPlayer.Name, newPlayer.Name)
				waitingPlayer.Conn.Write([]byte("MATCH_FOUND\n"))
				newPlayer.Conn.Write([]byte("MATCH_FOUND\n"))
				go handleGameSession(waitingPlayer, newPlayer)
				waitingPlayer = nil
				timeoutChan = nil
			}
		case <-timeoutChan:
			if waitingPlayer != nil {
				log.Printf("Timeout para %s. Nenhuma partida encontrada.", waitingPlayer.Name)
				waitingPlayer.Conn.Write([]byte("NO_MATCH_FOUND\n"))
				go handlePlayerConnection(waitingPlayer.Conn)
				waitingPlayer = nil
				timeoutChan = nil
			}
		}
	}
}

func handleGameSession(player1 *Player, player2 *Player) {
	atomic.AddInt64(&totalMatchesPlayed, 1)

	defer func() {
		go handlePlayerConnection(player1.Conn)
		go handlePlayerConnection(player2.Conn)
	}()

	log.Printf("Iniciando sessão de jogo entre %s e %s.", player1.Name, player2.Name)

	hand1 := selectRandomCards(player1.Deck, 2)
	hand2 := selectRandomCards(player2.Deck, 2)

	if hand1 == nil || hand2 == nil {
		msg := "Erro: Cartas insuficientes no deck para iniciar a partida.\n"
		player1.Conn.Write([]byte(msg))
		player2.Conn.Write([]byte(msg))
		return
	}

	p1HandStr := fmt.Sprintf("MATCH_START|%s (%d)|%s (%d)\n", hand1[0].Name, hand1[0].Forca, hand1[1].Name, hand1[1].Forca)
	p2HandStr := fmt.Sprintf("MATCH_START|%s (%d)|%s (%d)\n", hand2[0].Name, hand2[0].Forca, hand2[1].Name, hand2[1].Forca)
	player1.Conn.Write([]byte(p1HandStr))
	player2.Conn.Write([]byte(p2HandStr))

	timerMsg := fmt.Sprintf("TIMER|%d\n", int(gameTurnTimeout.Seconds()))
	player1.Conn.Write([]byte(timerMsg))
	player2.Conn.Write([]byte(timerMsg))

	session := &GameSession{Player1: player1, Player2: player2}
	cardPlayed := make(chan struct{}, 2)

	go listenForPlayerMove(session, player1, hand1, cardPlayed)
	go listenForPlayerMove(session, player2, hand2, cardPlayed)

	timeout := time.After(gameTurnTimeout)
	moves := 0
Loop:
	for {
		select {
		case <-cardPlayed:
			moves++
			if moves == 2 {
				break Loop
			}
		case <-timeout:
			log.Printf("Timeout da partida entre %s e %s.", player1.Name, player2.Name)
			break Loop
		}
	}

	determineWinner(session)
}

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

func listenForPlayerMove(session *GameSession, player *Player, hand []Card, cardPlayed chan struct{}) {
	player.Conn.SetReadDeadline(time.Now().Add(gameTurnTimeout + 2*time.Second))
	reader := bufio.NewReader(player.Conn)
	message, err := reader.ReadString('\n')
	player.Conn.SetReadDeadline(time.Time{})

	if err != nil {
		log.Printf("Jogador %s não fez uma jogada ou desconectou: %v", player.Name, err)
		return
	}

	choiceStr := strings.TrimSpace(message)
	choice, err := strconv.Atoi(choiceStr)
	if err != nil || (choice != 1 && choice != 2) {
		return
	}

	playedCard := hand[choice-1]

	session.mu.Lock()
	if player.Name == session.Player1.Name {
		session.Player1Card = &playedCard
	} else {
		session.Player2Card = &playedCard
	}
	session.mu.Unlock()

	log.Printf("%s jogou a carta: %s", player.Name, playedCard.Name)
	cardPlayed <- struct{}{}
}

func determineWinner(session *GameSession) {
	session.mu.Lock()
	defer session.mu.Unlock()

	p1Card := session.Player1Card
	p2Card := session.Player2Card

	var resultP1, resultP2 string
	var logMessage string // <-- Variável para a mensagem de log

	if p1Card != nil && p2Card != nil {
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
		resultP1 = "RESULT|DERROTA|Você não jogou a tempo e perdeu.\n"
		resultP2 = fmt.Sprintf("RESULT|VITÓRIA|%s não jogou a tempo. Você venceu!\n", session.Player1.Name)
		logMessage = fmt.Sprintf("Resultado: %s venceu %s por timeout.", session.Player2.Name, session.Player1.Name)
	} else if p2Card == nil && p1Card != nil {
		resultP2 = "RESULT|DERROTA|Você não jogou a tempo e perdeu.\n"
		resultP1 = fmt.Sprintf("RESULT|VITÓRIA|%s não jogou a tempo. Você venceu!\n", session.Player2.Name)
		logMessage = fmt.Sprintf("Resultado: %s venceu %s por timeout.", session.Player1.Name, session.Player2.Name)
	} else {
		result := "RESULT|EMPATE|Nenhum jogador jogou a tempo. Empate.\n"
		resultP1, resultP2 = result, result
		logMessage = fmt.Sprintf("Resultado: Empate por timeout duplo entre %s e %s.", session.Player1.Name, session.Player2.Name)
	}

	session.Player1.Conn.Write([]byte(resultP1))
	session.Player2.Conn.Write([]byte(resultP2))

	// --- LINHA ADICIONADA ---
	log.Printf("Partida entre %s e %s finalizada. %s", session.Player1.Name, session.Player2.Name, logMessage)
}

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
		currentTimestamp := time.Now().UnixNano()
		latency := (currentTimestamp - sentTimestamp) / int64(time.Microsecond)
		fmt.Printf("Pacote UDP de %s recebido (%d µs de latência)\n", playerName, latency)
	}
}
