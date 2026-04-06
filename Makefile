.PHONY: build start

build:
	go build -o bin/go-trade .

start: build
	./bin/go-trade live