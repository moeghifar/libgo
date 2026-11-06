.PHONY: check verify vet test lint security vuln trivy secrets docker docker-run

GOLANGCI := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
VERSION := $(shell git describe --tags --always --dirty)
GOVULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@latest
GITLEAKS := go run github.com/zricethezav/gitleaks/v8@v8.30.1
TRIVY := docker run --rm -v $(CURDIR):/src:ro -v trivy-cache:/root/.cache/trivy aquasec/trivy:0.76.1
# Split out of .golangci.yml so lint and security never report the same finding.
SECURITY_LINTERS := gosec,bidichk,bodyclose,noctx,reassign,rowserrcheck,sqlclosecheck

check: verify vet test lint security vuln trivy secrets

verify:
	go mod verify

vet:
	go vet ./...

test:
	go test -race ./...

lint:
	$(GOLANGCI) run --disable $(SECURITY_LINTERS) ./...

security:
	$(GOLANGCI) run --enable-only $(SECURITY_LINTERS) ./...

vuln:
	$(GOVULNCHECK) ./...

# Dependencies, secrets and misconfiguration in the working tree. .env and .specify stay local.
trivy:
	$(TRIVY) fs --exit-code 1 --scanners vuln,secret,misconfig --skip-dirs .specify --skip-files .env /src

# Secrets anywhere in git history, which trivy does not read.
secrets:
	$(GITLEAKS) git --redact --no-banner .

docker:
	docker build -f example/Dockerfile --build-arg APP_VERSION=$(VERSION) -t libgo-example .

# Isolated run: read-only root, no capabilities, no privilege gain, bounded resources.
docker-run: docker
	docker run --rm -p 127.0.0.1:8080:8080 --read-only --cap-drop ALL \
		--security-opt no-new-privileges --pids-limit 64 --memory 128m --cpus 0.5 \
		libgo-example
