package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

/*const (
	serverTcpAddr = "server:8080"
	serverUdpAddr = "server:8081"
)*/

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Uso: ./client <nome_do_jogador> <ip_do_servidor>")
	}
	playerName := os.Args[1]
	serverIP := os.Args[2]

	serverTcpAddr := fmt.Sprintf("%s:8080", serverIP)
    serverUdpAddr := fmt.Sprintf("%s:8081", serverIP)


	done := make(chan bool)
	go startMatchmaking(playerName, serverTcpAddr, done)
	go sendUdpData(playerName, serverUdpAddr)

	<-done
	fmt.Printf("%s: Encerrando cliente.\n", playerName)
}

func startMatchmaking(playerName string, serverTcpAddr string, done chan bool) {
	defer func() { done <- true }()

	log.Printf("%s: Conectando ao servidor TCP (%s) para pareamento...", playerName, serverTcpAddr)
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

	fmt.Printf("%s: Conectado. Aguardando por um pareamento...\n", playerName)

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		log.Printf("%s: Conexão com o servidor encerrada. Motivo: %v", playerName, err)
		return
	}

	response := string(buffer[:n])

	switch response {
	case "MATCH_FOUND":
		fmt.Printf(" %s: Partida encontrada! Preparando para jogar...\n", playerName)
		time.Sleep(5 * time.Second)
	case "NO_MATCH_FOUND":
		fmt.Printf(" %s: Não foi possível encontrar uma partida. Tente novamente mais tarde.\n", playerName)
	default:
		fmt.Printf("%s: Resposta inesperada do servidor: %s\n", playerName, response)
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
			return
		}
		time.Sleep(2 * time.Second)
	}
}
