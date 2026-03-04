.PHONY: build run test test-v test-fresh vet lint clean docker docker-run compare compare-golive compare-rediff compare-clean compare-go baseline-build baseline-up baseline-down kill-port

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

compare-golive:
	go run ./cmd/comparator golive

compare-rediff:
	go run ./cmd/comparator rediff

compare-clean:
	go run ./cmd/comparator clean

baseline-build: ## Build and tag current Go app as baseline
	docker build -t f4-baseline:latest .

baseline-up: ## Start baseline Go app on port 8081
	docker compose -f docker-compose.baseline.yml up -d

baseline-down: ## Stop baseline
	docker compose -f docker-compose.baseline.yml down

compare-go: ## Compare baseline Go vs dev Go
	go run ./cmd/comparator gocheck

kill-port:
	go run scripts/killport.go

