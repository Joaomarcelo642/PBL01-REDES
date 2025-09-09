#!/bin/bash

# Verifica se o número de jogadores foi passado como argumento
if [ -z "$1" ] || [ "$1" -le 0 ]; then
  echo "Uso: ./gerar_teste.sh <numero_de_jogadores>"
  echo "Exemplo: ./gerar_teste.sh 10"
  exit 1
fi

N=$1
dockerComposeFile="docker-compose.${N}-players.yml"

# Inicia o conteúdo com o serviço do servidor
cat > "$dockerComposeFile" <<EOF
version: '3.8'

services:
  server:
    build:
      context: .
      dockerfile: Dockerfile.server
    ports:
      - "8080:8080" # Porta TCP
      - "8081:8081/udp" # Porta UDP
    networks:
      - app-network
    restart: unless-stopped
EOF

# Adiciona os serviços dos jogadores
for i in $(seq 1 $N); do
cat >> "$dockerComposeFile" <<EOF

  player${i}:
    build:
      context: .
      dockerfile: Dockerfile.client
    depends_on:
      - server
    command: ["./client", "-bot", "Jogador${i}", "server"]
    networks:
      - app-network
    stdin_open: true 
    tty: true
    restart: on-failure
EOF
done

# Adiciona a configuração de rede
cat >> "$dockerComposeFile" <<EOF

networks:
  app-network:
    driver: bridge
EOF

# Mensagens finais
echo "Arquivo '${dockerComposeFile}' gerado para ${N} jogadores em MODO AUTOMATIZADO."
echo ""
echo "Para iniciar os testes, execute:"
echo "docker-compose -f ${dockerComposeFile} up --build"
echo ""
echo "Os jogadores irão se conectar, jogar uma partida e desconectar automaticamente."
echo "Você pode observar a ação com o comando:"
echo "docker-compose -f ${dockerComposeFile} logs -f"
echo ""
echo "Para parar e remover os containers, execute:"
echo "docker-compose -f ${dockerComposeFile} down"
