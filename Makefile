APP=golivepilot
IMAGE=golivepilot:dev
COMPOSE=docker compose

.PHONY: run test build docker-build docker-up docker-down logs

run:
	go run ./cmd/$(APP)

test:
	go test ./...

build:
	go build -o bin/$(APP) ./cmd/$(APP)

docker-build:
	docker build -t $(IMAGE) .

docker-up:
	$(COMPOSE) up -d --build

docker-down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f
