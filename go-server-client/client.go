package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

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
	log.Printf("%s: Conectando ao servidor TCP (%s)...", playerName, serverTcpAddr)
	conn, err := net.Dial("tcp", serverTcpAddr)
	if err != nil {
		log.Printf("%s: Não foi possível conectar ao servidor TCP: %v", playerName, err)
		return
	}
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "%s\n", playerName)
	if err != nil {
		log.Printf("%s: Falha ao enviar nome para o servidor: %v", playerName, err)
		return
	}
	fmt.Printf("%s: Conectado com sucesso!\n", playerName)

	go listenServerMessages(conn, playerName)

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("\nEscolha uma ação:")
		fmt.Println("1. Procurar Partida (1v1)")
		fmt.Println("2. Abrir Pacote de Cartas")
		fmt.Println("3. Sair")
		fmt.Print("> ")

		input, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			fmt.Fprintf(conn, "FIND_MATCH\n")
			fmt.Println("Procurando por uma partida... Aguarde a resposta do servidor.")
			// Bloqueia a goroutine principal aqui. A goroutine listenServerMessages
			// vai chamar os.Exit() quando a conexão for fechada pelo servidor.
			select {}
		case "2":
			fmt.Fprintf(conn, "OPEN_PACK\n")
			// Dá um tempo para a resposta do servidor ser impressa antes de mostrar o menu
			time.Sleep(1 * time.Second)
		case "3":
			return
		default:
			fmt.Println("Opção inválida. Tente novamente.")
		}
	}
}

func listenServerMessages(conn net.Conn, playerName string) {
	reader := bufio.NewReader(conn)
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("%s: Conexão com o servidor perdida.", playerName)
			os.Exit(0) // Encerra o cliente
		}
		// Limpa a linha atual e imprime a mensagem do servidor
		fmt.Printf("\r[Servidor]: %s\n> ", strings.TrimSpace(message))
	}
}

func sendUdpData(playerName string, serverUdpAddr string) {
	udpAddr, err := net.ResolveUDPAddr("udp", serverUdpAddr)
	if err != nil {
		log.Printf("%s: Não foi possível resolver o endereço do servidor UDP: %v", playerName, err)
		return
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		log.Printf("%s: Não foi possível conectar ao servidor UDP: %v", playerName, err)
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
