VERSION ?= $(shell git describe --tags --always --dirty)

docker-build:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--push \
		--tag wang/postgres-mcp-sse:$(VERSION) \
		--tag wang/postgres-mcp-sse:latest \
		.