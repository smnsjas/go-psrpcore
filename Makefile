# Makefile for go-psrpcore performance optimization

.PHONY: help bench bench-baseline bench-save bench-compare test test-race profile clean

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

# Testing
test: ## Run all tests
	go test ./...

test-race: ## Run tests with race detector
	go test -race ./...

test-verbose: ## Run tests with verbose output
	go test -v ./...

# Benchmarking
bench: ## Run all benchmarks
	go test -bench=. -benchmem -run=^$$ ./test/...

bench-baseline: ## Run benchmarks and save baseline
	@echo "Running baseline benchmarks..."
	@mkdir -p test/benchmarks
	go test -bench=. -benchmem -count=5 -run=^$$ ./test/... | tee test/benchmarks/baseline.txt
	@echo "Baseline saved to test/benchmarks/baseline.txt"

bench-save: ## Run benchmarks and save results with timestamp
	@echo "Running benchmarks..."
	@mkdir -p test/benchmarks
	@TIMESTAMP=$$(date +%Y%m%d_%H%M%S); \
	go test -bench=. -benchmem -count=5 -run=^$$ ./test/... | tee test/benchmarks/bench_$$TIMESTAMP.txt
	@echo "Results saved to test/benchmarks/bench_$$TIMESTAMP.txt"

bench-compare: ## Compare latest benchmark with baseline (requires benchstat)
	@if [ ! -f test/benchmarks/baseline.txt ]; then \
		echo "Error: No baseline found. Run 'make bench-baseline' first."; \
		exit 1; \
	fi
	@LATEST=$$(ls -t test/benchmarks/bench_*.txt 2>/dev/null | head -1); \
	if [ -z "$$LATEST" ]; then \
		echo "Error: No benchmark results found. Run 'make bench-save' first."; \
		exit 1; \
	fi; \
	echo "Comparing baseline with $$LATEST"; \
	benchstat test/benchmarks/baseline.txt $$LATEST

# Profiling
profile-cpu: ## Run CPU profiling
	@mkdir -p test/profiles
	go test -bench=BenchmarkRoundTripMedium -cpuprofile=test/profiles/cpu.prof -run=^$$ ./test/...
	@echo "CPU profile saved to test/profiles/cpu.prof"
	@echo "View with: go tool pprof -http=:8080 test/profiles/cpu.prof"

profile-mem: ## Run memory profiling
	@mkdir -p test/profiles
	go test -bench=BenchmarkRoundTripMedium -memprofile=test/profiles/mem.prof -run=^$$ ./test/...
	@echo "Memory profile saved to test/profiles/mem.prof"
	@echo "View with: go tool pprof -http=:8080 test/profiles/mem.prof"

profile-all: ## Run both CPU and memory profiling
	@mkdir -p test/profiles
	go test -bench=BenchmarkRoundTripMedium -cpuprofile=test/profiles/cpu.prof -memprofile=test/profiles/mem.prof -run=^$$ ./test/...
	@echo "Profiles saved to test/profiles/"
	@echo "View CPU: go tool pprof -http=:8080 test/profiles/cpu.prof"
	@echo "View MEM: go tool pprof -http=:8081 test/profiles/mem.prof"

# Analysis
escape: ## Run escape analysis
	go build -gcflags="-m -m" ./... 2>&1 | grep "escapes to heap"

fieldalignment: ## Check struct field alignment
	@which fieldalignment > /dev/null || (echo "Installing fieldalignment..." && go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest)
	fieldalignment ./...

staticcheck: ## Run static analysis
	@which staticcheck > /dev/null || (echo "Installing staticcheck..." && go install honnef.co/go/tools/cmd/staticcheck@latest)
	staticcheck ./...

vet: ## Run go vet
	go vet ./...

# Build
build: ## Build the project
	go build ./...

build-release: ## Build optimized release binary
	go build -ldflags="-s -w" -trimpath -o bin/psrp-client ./cmd/psrp-client

build-pgo: ## Build with profile-guided optimization (requires test/profiles/default.pprof)
	@if [ ! -f test/profiles/default.pprof ]; then \
		echo "Error: No PGO profile found. Run 'make profile-cpu' first and copy test/profiles/cpu.prof to test/profiles/default.pprof"; \
		exit 1; \
	fi
	go build -pgo=test/profiles/default.pprof -o bin/psrp-client ./cmd/psrp-client

# Cleanup
clean: ## Clean build artifacts and benchmark results
	rm -rf bin/
	rm -rf test/benchmarks/
	rm -rf test/profiles/
	go clean -cache -testcache

# Installation
install-tools: ## Install required tools (benchstat, fieldalignment, staticcheck)
	@echo "Installing Go performance tools..."
	go install golang.org/x/perf/cmd/benchstat@latest
	go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
	@echo "Tools installed successfully!"

# Quick workflow targets
quick-bench: ## Quick benchmark run (count=1)
	go test -bench=. -benchmem -count=1 -run=^$$ ./...

verify: test test-race vet ## Run all verification steps

full-check: verify staticcheck bench ## Run complete validation and benchmarking
