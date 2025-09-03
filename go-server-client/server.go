package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tcpPort            = ":8080"
	udpPort            = ":8081"
	matchmakingTimeout = 15 * time.Second
)

type Player struct {
	Name string
	Conn net.Conn
}

var (
	matchmakingQueue = make(chan Player)
	cardPacks        = 10 
	stockMutex       = &sync.Mutex{}
)

func main() {
	go startTcpServer()
	go matchmaker()
	go startUdpServer()

	fmt.Println("Servidor iniciado. Aguardando clientes...")
	select {}
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
	reader := bufio.NewReader(conn)
	name, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("Erro ao ler o nome do jogador de %s: %v", conn.RemoteAddr(), err)
		conn.Close()
		return
	}
	name = strings.TrimSpace(name)
	player := Player{Name: name, Conn: conn}
	log.Printf("Jogador %s conectado de %s", player.Name, player.Conn.RemoteAddr())

	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Jogador %s desconectado.", player.Name)
			conn.Close()
			return
		}

		command := strings.TrimSpace(message)
		log.Printf("Comando recebido de %s: %s", player.Name, command)

		switch command {
		case "FIND_MATCH":
			matchmakingQueue <- player
			return
		case "OPEN_PACK":
			openCardPack(player)
		default:
			player.Conn.Write([]byte("Comando inválido.\n"))
		}
	}
}

func openCardPack(player Player) {
	stockMutex.Lock()
	defer stockMutex.Unlock()

	if cardPacks > 0 {
		cardPacks--
		response := fmt.Sprintf("Parabéns, %s! Você abriu um pacote. Pacotes restantes: %d\n", player.Name, cardPacks)
		player.Conn.Write([]byte(response))
	} else {
		response := "Desculpe, não há mais pacotes de cartas disponíveis.\n"
		player.Conn.Write([]byte(response))
	}
}

func matchmaker() {
	var waitingPlayer *Player
	var timer *time.Timer
	var timeoutChan <-chan time.Time

	for {
		select {
		case newPlayer := <-matchmakingQueue:
			fmt.Printf("Jogador %s entrou na fila de pareamento...\n", newPlayer.Name)

			if waitingPlayer == nil {
				waitingPlayer = &newPlayer
				fmt.Printf("Jogador %s está aguardando um oponente. Iniciando timeout de %v.\n", waitingPlayer.Name, matchmakingTimeout)
				timer = time.NewTimer(matchmakingTimeout)
				timeoutChan = timer.C
			} else {
				fmt.Printf("Pareamento encontrado! %s vs %s\n", waitingPlayer.Name, newPlayer.Name)
				timer.Stop()
				timeoutChan = nil

				matchMessage := "MATCH_FOUND\n"
				waitingPlayer.Conn.Write([]byte(matchMessage))
				newPlayer.Conn.Write([]byte(matchMessage))

				// Inicia uma nova goroutine para a sessão de jogo
				go handleGameSession(*waitingPlayer, newPlayer)

				waitingPlayer = nil
			}

		case <-timeoutChan:
			if waitingPlayer != nil {
				fmt.Printf("Timeout para o jogador %s. Nenhuma partida encontrada.\n", waitingPlayer.Name)
				timeoutMessage := "NO_MATCH_FOUND\n"
				waitingPlayer.Conn.Write([]byte(timeoutMessage))
				waitingPlayer.Conn.Close()

				waitingPlayer = nil
				timeoutChan = nil
			}
		}
	}
}

// Função para gerenciar a partida
func handleGameSession(player1 Player, player2 Player) {
	// Garante que as conexões dos jogadores sejam fechadas ao final da partida
	defer player1.Conn.Close()
	defer player2.Conn.Close()

	log.Printf("Iniciando sessão de jogo entre %s e %s.", player1.Name, player2.Name)
	player1.Conn.Write([]byte("A partida vai começar!\n"))
	player2.Conn.Write([]byte("A partida vai começar!\n"))

	time.Sleep(20 * time.Second)

	log.Printf("Encerrando sessão de jogo entre %s e %s.", player1.Name, player2.Name)
	player1.Conn.Write([]byte("A partida terminou.\n"))
	player2.Conn.Write([]byte("A partida terminou.\n"))
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
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Erro ao ler pacote UDP: %v", err)
			continue
		}
		message := string(buffer[:n])
		parts := strings.Split(message, ":")
		if len(parts) != 2 {
			log.Printf("Formato de pacote UDP inválido de %s", addr)
			continue
		}

		playerName := parts[0]
		sentTimestamp, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			log.Printf("Timestamp inválido de %s", addr)
			continue
		}

		currentTimestamp := time.Now().UnixNano()
		latency := (currentTimestamp - sentTimestamp) / int64(time.Microsecond)
		fmt.Printf("Pacote UDP de %s recebido (%d µs de latência)\n", playerName, latency)
	}
}

