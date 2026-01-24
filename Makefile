.PHONY: server client clean

server:
	@echo "Building Server..."
	cd server && go build -o ../bin/server .
	@echo "Server built at bin/server"

client:
	@echo "Building Client..."
	cd client && go build -o ../bin/client .
	@echo "Client built at bin/client"

run-server: server
	./bin/server

run-client: client
	./bin/client

clean:
	rm -rf bin
