.PHONY: build frontend-build go-build dev clean

build: frontend-build go-build

frontend-build:
	cd frontend && npm run build
	rm -rf internal/server/dist
	cp -r frontend/dist internal/server/dist

go-build:
	go build -o rdq ./cmd/rdq/

dev:
	cd frontend && npm run dev

clean:
	rm -f rdq
	rm -rf frontend/dist internal/server/dist
