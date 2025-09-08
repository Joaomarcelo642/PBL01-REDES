# Verifica se o número de jogadores foi passado como argumento
param (
    [int]$N
)

if (-not $N -or $N -le 0) {
    Write-Host "Uso: .\gerar_teste.ps1 -N <numero_de_jogadores>"
    Write-Host "Exemplo: .\gerar_teste.ps1 -N 10"
    exit 1
}

$dockerComposeFile = "docker-compose.${N}-players.yml"

# Inicia o arquivo docker-compose com o serviço do servidor
$serverContent = @"
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
    # Adicionado para reiniciar o servidor em caso de falha
    restart: unless-stopped

"@

Set-Content -Path $dockerComposeFile -Value $serverContent

# Adiciona os serviços dos jogadores ao arquivo
1..$N | ForEach-Object {
    $playerContent = @"

  player${_}:
    build:
      context: .
      dockerfile: Dockerfile.client
    depends_on:
      - server
    # ATUALIZAÇÃO: Adicionada a flag "-bot" para iniciar em modo automatizado
    command: ["./client", "-bot", "Jogador${_}", "server"]
    networks:
      - app-network
    # stdin_open e tty não são mais necessários para o bot, mas não atrapalham
    stdin_open: true 
    tty: true
    # Adicionado para que o bot não reinicie após terminar a partida
    restart: on-failure
"@
    Add-Content -Path $dockerComposeFile -Value $playerContent
}

# Adiciona a configuração de rede ao final do arquivo
$networkContent = @"

networks:
  app-network:
    driver: bridge
"@

Add-Content -Path $dockerComposeFile -Value $networkContent

Write-Host "Arquivo '${dockerComposeFile}' gerado para ${N} jogadores em MODO AUTOMATIZADO."
Write-Host ""
Write-Host "Para iniciar os testes, execute:"
Write-Host "docker-compose -f ${dockerComposeFile} up --build"
Write-Host ""
Write-Host "Os jogadores irão se conectar, jogar uma partida e desconectar automaticamente."
Write-Host "Você pode observar a ação com o comando:"
Write-Host "docker-compose -f ${dockerComposeFile} logs -f"
Write-Host ""
Write-Host "Para parar e remover os containers, execute:"
Write-Host "docker-compose -f ${dockerComposeFile} down"