package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
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

var matchmakingQueue = make(chan Player)

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
	log.Println("Servidor TCP de pareamento iniciado na porta", tcpPort)

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
	matchmakingQueue <- player
}

func matchmaker() {
	var waitingPlayer *Player
	var timer *time.Timer
	var timeoutChan <-chan time.Time

	for {
		select {
		case newPlayer := <-matchmakingQueue:
			fmt.Printf("Novo jogador conectado: %s (%s). Verificando fila...\n", newPlayer.Name, newPlayer.Conn.RemoteAddr())

			if waitingPlayer == nil {
				// Armazena o ponteiro do novo jogador.
				waitingPlayer = &newPlayer
				fmt.Printf("Jogador %s está na fila. Iniciando timeout de %v.\n", waitingPlayer.Name, matchmakingTimeout)

				timer = time.NewTimer(matchmakingTimeout)
				timeoutChan = timer.C

			} else {
				fmt.Printf("Pareamento encontrado! %s vs %s\n", waitingPlayer.Name, newPlayer.Name)

				timer.Stop()
				timeoutChan = nil

				matchMessage := "MATCH_FOUND"
				waitingPlayer.Conn.Write([]byte(matchMessage))
				newPlayer.Conn.Write([]byte(matchMessage))

				waitingPlayer.Conn.Close()
				newPlayer.Conn.Close()

				waitingPlayer = nil
			}

		case <-timeoutChan:
			fmt.Printf("Timeout para o jogador %s. Nenhuma partida encontrada.\n", waitingPlayer.Name)

			timeoutMessage := "NO_MATCH_FOUND"
			waitingPlayer.Conn.Write([]byte(timeoutMessage))
			waitingPlayer.Conn.Close()

			// Reseta o estado.
			waitingPlayer = nil
			timeoutChan = nil
		}
	}
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