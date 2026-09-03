.PHONY: run-backend run-web tidy web-build

tidy:
	go mod tidy

web-build:
	cd web && npm install && npm run build

run-backend:
	go run ./cmd/netlynxd

run-web:
	cd web && npm install && npm run dev
