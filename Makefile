.PHONY: build demo serve dev-api dev-web test check image

build:
	cd web && npm ci && npm run build
	CGO_ENABLED=0 go build -trimpath -o bin/wiki ./cmd/wiki

demo: build
	./bin/wiki serve --demo --data-dir ./data/demo --listen 127.0.0.1:8080

serve: build
	./bin/wiki serve

dev-api:
	go run ./cmd/wiki serve --demo --data-dir ./data/demo

dev-web:
	cd web && npm run dev

test:
	go test -race ./...

check:
	go vet ./...
	cd web && npm run check

image: build
	CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o bin/wiki-linux ./cmd/wiki
	mkdir -p bin/empty-data
	docker build -f Dockerfile.binary -t niaotuobang-wiki:local bin
