.PHONY: build run test test-v test-fresh vet lint clean docker compare compare-clean

APP_NAME := f4
BUILD_DIR := ./cmd/server

build:
	go build -o $(APP_NAME) $(BUILD_DIR)

run: build
	./$(APP_NAME)

test:
	go test ./...

test-v:
	go test -v -count=1 ./...

test-fresh:
	go test -count=1 ./...

vet:
	go vet ./...

lint: vet
	@echo "Lint passed"

clean:
	rm -f $(APP_NAME)

docker:
	docker build -t $(APP_NAME) .

docker-run: docker
	docker run --rm -p 8080:8080 --env-file .env $(APP_NAME)

compare:
	go run ./cmd/comparator run

compare-clean:
	go run ./cmd/comparator clean
