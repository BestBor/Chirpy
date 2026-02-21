build:
	@go build -o out && ./out

run: build
	@./out

test: 
	@go test ./... -v

