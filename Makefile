# Makefile for building and running the Go project

BINARY_NAME=sequence-game

build:
	go build -o $(BINARY_NAME) main.go

run:
	go run main.go
#	./$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)
