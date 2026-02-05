.PHONY: server client windows-server linux-arm-client linux-arm64-client tizen-client clean

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
	@echo "Building Client (Linux ARM 32-bit/Raspberry Pi)..."
	cd client && GOOS=linux GOARCH=arm go build -o ../bin/client-arm .
	@echo "Client built at bin/client-arm"

linux-arm64-client:
	@echo "Building Client (Linux ARM 64-bit/Raspberry Pi)..."
	cd client && GOOS=linux GOARCH=arm64 go build -o ../bin/client-arm64 .
	@echo "Client built at bin/client-arm64"

tizen-client:
	@echo "Packaging Tizen Client (.wgt)..."
	mkdir -p bin
	cd client-tizen && zip -r ../bin/client-tizen.wgt . -x "docs/*"
	@echo "Tizen Client packaged at bin/client-tizen.wgt"

run-server: server
	./bin/server

run-client: client
	./bin/client

clean:
	rm -rf bin