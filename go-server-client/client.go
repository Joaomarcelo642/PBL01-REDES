package main

import (
	"bufio"
	"context"
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

var stateMutex sync.Mutex
var isSearching bool
var isInGame bool

const matchmakingTimeoutSeconds = 15

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Uso: ./client <nome_do_jogador> <ip_do_servidor>")
	}
	playerName := os.Args[1]
	serverIP := os.Args[2]

	serverTcpAddr := fmt.Sprintf("%s:8080", serverIP)
	serverUdpAddr := fmt.Sprintf("%s:8081", serverIP)

	go sendUdpData(playerName, serverUdpAddr)
	handleServerConnection(playerName, serverTcpAddr)
	fmt.Printf("%s: Encerrando cliente.\n", playerName)
}

func handleServerConnection(playerName string, serverTcpAddr string) {
	var conn net.Conn
	var err error

	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		conn, err = net.Dial("tcp", serverTcpAddr)
		if err == nil {
			break
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

	go listenServerMessages(conn, playerName, cancelGame)

	reader := bufio.NewReader(os.Stdin)
	for {
		stateMutex.Lock()
		canShowMenu := !isSearching && !isInGame
		stateMutex.Unlock()

		if canShowMenu {
			showMenu()
			input, _ := reader.ReadString('\n')
			choice := strings.TrimSpace(input)

			switch choice {
			case "1":
				stateMutex.Lock()
				isSearching = true
				stateMutex.Unlock()
				fmt.Fprintf(conn, "FIND_MATCH\n")
				go runSearchCountdown(matchmakingTimeoutSeconds)
			case "2":
				fmt.Fprintf(conn, "OPEN_PACK\n")
			case "3":
				fmt.Fprintf(conn, "VIEW_DECK\n")
			case "4":
				return
			default:
				fmt.Println("Opção inválida. Tente novamente.")
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func showMenu() {
	fmt.Println("\n--- MENU PRINCIPAL ---")
	fmt.Println("1. Procurar Partida")
	fmt.Println("2. Abrir Pacote de Cartas")
	fmt.Println("3. Ver Meu Deck")
	fmt.Println("4. Sair")
	fmt.Print("> ")
}

func listenServerMessages(conn net.Conn, playerName string, cancelGame context.CancelFunc) {
	reader := bufio.NewReader(conn)
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				log.Printf("%s: O servidor fechou a conexão.", playerName)
			} else {
				log.Printf("%s: Conexão com o servidor perdida: %v", playerName, err)
			}
			os.Exit(0)
		}

		message = strings.TrimSpace(message)
		fmt.Printf("\r%s\n", strings.Repeat(" ", 50))

		if strings.HasPrefix(message, "MATCH_START|") {
			stateMutex.Lock()
			isSearching = false
			isInGame = true
			stateMutex.Unlock()

			gameCtx, newCancel := context.WithCancel(context.Background())
			cancelGame = newCancel

			handleGame(gameCtx, conn, message)
		} else if strings.HasPrefix(message, "RESULT|") {
			cancelGame()
			parts := strings.SplitN(message, "|", 2)
			fmt.Printf("\r--- FIM DA PARTIDA ---\n%s\n---------------------\n", parts[1])
			stateMutex.Lock()
			isInGame = false
			stateMutex.Unlock()
		} else if message == "MATCH_FOUND" {
			fmt.Printf("\r[Servidor]: Partida encontrada! Iniciando...\n")
			stateMutex.Lock()
			isSearching = false 
			stateMutex.Unlock()
		} else if message == "NO_MATCH_FOUND" {
			fmt.Printf("\r[Servidor]: Nenhum oponente encontrado a tempo. Tente novamente.\n")
			stateMutex.Lock()
			isSearching = false 
			stateMutex.Unlock()
		} else if strings.HasPrefix(message, "TIMER|") {
			parts := strings.Split(message, "|")
			seconds, _ := strconv.Atoi(parts[1])
			go runGameCountdown(seconds)
		} else {
			fmt.Printf("\r[Servidor]: %s\n", message)
		}

		stateMutex.Lock()
		if !isSearching && !isInGame {
			fmt.Print("> ")
		}
		stateMutex.Unlock()
	}
}

func handleGame(ctx context.Context, conn net.Conn, message string) {
	parts := strings.Split(message, "|")
	card1 := parts[1]
	card2 := parts[2]

	fmt.Println("\r--- PARTIDA INICIADA ---")
	fmt.Println("Sua mão:")
	fmt.Printf("1: %s\n", card1)
	fmt.Printf("2: %s\n", card2)
	fmt.Print("Escolha sua carta (1 ou 2): > ")

	go readPlayerInput(ctx, conn)
}

func readPlayerInput(ctx context.Context, conn net.Conn) {
	choiceChan := make(chan string)
	reader := bufio.NewReader(os.Stdin)

	go func() {
		input, err := reader.ReadString('\n')
		if err == nil {
			choiceChan <- strings.TrimSpace(input)
		}
	}()

	select {
	case choice := <-choiceChan:
		fmt.Fprintf(conn, "%s\n", choice)
		fmt.Println("Jogada enviada. Aguardando resultado...")
	case <-ctx.Done():
		fmt.Println("\nA partida terminou antes de você fazer uma jogada.")
		return
	}
}

func runSearchCountdown(seconds int) {
	for i := seconds; i > 0; i-- {
		stateMutex.Lock()
		if !isSearching {
			stateMutex.Unlock()
			fmt.Printf("\r%s\r", strings.Repeat(" ", 50)) // Limpa a linha
			return
		}
		stateMutex.Unlock()

		fmt.Printf("\rBuscando partida... Tempo restante: %d segundos ", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Printf("\r%s\r", strings.Repeat(" ", 50))
}

func runGameCountdown(seconds int) {
	for i := seconds; i > 0; i-- {
		stateMutex.Lock()
		if !isInGame {
			stateMutex.Unlock()
			fmt.Printf("\r%s\r", strings.Repeat(" ", 50))
			return
		}
		stateMutex.Unlock()

		fmt.Printf("\rTempo de jogada restante: %d segundos... ", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Printf("\r%s\r", strings.Repeat(" ", 50))
}

func sendUdpData(playerName string, serverUdpAddr string) {
	var conn net.Conn
	var err error

	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		udpAddr, err := net.ResolveUDPAddr("udp", serverUdpAddr)
		if err != nil {
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

	for {
		timestamp := time.Now().UnixNano()
		message := fmt.Sprintf("%s:%d", playerName, timestamp)
		_, err := conn.Write([]byte(message))
		if err != nil {
			log.Printf("%s: Erro ao enviar pacote UDP: %v", playerName, err)
		}
		time.Sleep(2 * time.Second)
	}
}
