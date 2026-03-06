BINARY=chirpy
BIN_DIR=bin

build:
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/$(BINARY)

run:
	@go run .

run-bin: build
	@./$(BIN_DIR)/$(BINARY)

test:
	@go test ./... -v

clean:
	@rm -rf $(BIN_DIR)