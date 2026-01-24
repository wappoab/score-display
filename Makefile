.PHONY: server client windows-server linux-arm-client clean

server:
	@echo "Building Server (Linux)..."
	cd server && go build -o ../bin/server .
	@echo "Server built at bin/server"

client:
	@echo "Building Client (Linux)..."
	cd client && go build -o ../bin/client .
	@echo "Client built at bin/client"

windows-server:
	@echo "Building Server (Windows)..."
	cd server && GOOS=windows GOARCH=amd64 go build -o ../bin/server.exe .
	@echo "Server built at bin/server.exe"

linux-arm-client:
	@echo "Building Client (Linux ARM/Raspberry Pi)..."
	cd client && GOOS=linux GOARCH=arm go build -o ../bin/client-arm .
	@echo "Client built at bin/client-arm"

run-server: server
	./bin/server

run-client: client
	./bin/client

clean:
	rm -rf bin