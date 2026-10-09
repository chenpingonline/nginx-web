FRONTEND_MODE ?= standalone
PERMISSION_MODE ?= full-ports
DOCKER_IMAGE ?= nginx-web:docker

.PHONY: frontend-install frontend-typecheck frontend-build test integration build-x86 build-arm64 build-all docker-build docker-test clean
frontend-install:
	npm --prefix web ci
frontend-typecheck:
	npm --prefix web run typecheck
frontend-build:
	@test -d web/node_modules || npm --prefix web ci
	FNPROXY_FRONTEND_MODE=$(FRONTEND_MODE) FNPROXY_PERMISSION_MODE=$(PERMISSION_MODE) npm --prefix web run build
test: frontend-build
	go test ./...
	go test -tags full_ports ./...
integration:
	./tests/integration.sh
build-x86:
	./linux/build.sh amd64
build-arm64:
	./linux/build.sh arm64
build-all:
	./linux/build.sh amd64 arm64
docker-build:
	docker build -f docker/Dockerfile -t $(DOCKER_IMAGE) .
docker-test:
	python3 docker/tests/integration.py --image $(DOCKER_IMAGE)
clean:
	rm -rf .build .cache dist docker/dist web/dist .fnproxy-dev
