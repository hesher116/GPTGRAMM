mkdir -p GPTGRAMM/cmd
mkdir -p GPTGRAMM/internal/{bot,api,storage,config}
cd GPTGRAMM
go mod init GPTGRAMM
touch .env
go get github.com/GPTGRAMM/telegram-bot-api/v5
go get github.com/joho/godotenv
go get modernc.org/sqlite 