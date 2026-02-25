build:
	@go build -o Chirpy && ./Chirpy

run: build
	@./Chirpy

test: 
	@go test ./... -v

