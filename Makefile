.PHONY: build run clean

build:
	@mkdir -p bin
	go build -o bin/sentinel-proxy-go main.go

run: build
	./bin/sentinel-proxy-go

test:
	go test -v ./...

clean:
	rm -rf bin

watch:
	@if [ -f "$$HOME/go/bin/air" ]; then \
		"$$HOME/go/bin/air"; \
	elif command -v air > /dev/null; then \
		air; \
	else \
		echo "Air not found. Run: go install github.com/air-verse/air@latest"; \
		exit 1; \
	fi
