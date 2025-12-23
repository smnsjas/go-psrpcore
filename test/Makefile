# Makefile for go-psrpcore performance testing
# Run from the test/ directory: cd test && make <target>

.PHONY: help bench bench-baseline bench-save bench-compare test test-race profile clean

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

# Testing
test: ## Run all tests
	cd .. && go test ./...

test-race: ## Run tests with race detector
	cd .. && go test -race ./...

test-verbose: ## Run tests with verbose output
	cd .. && go test -v ./...

# Benchmarking
bench: ## Run all benchmarks
	go test -bench=. -benchmem -run=^$$ ./...

bench-baseline: ## Run benchmarks and save baseline
	@echo "Running baseline benchmarks..."
	@mkdir -p benchmarks
	go test -bench=. -benchmem -count=5 -run=^$$ ./... | tee benchmarks/baseline.txt
	@echo "Baseline saved to benchmarks/baseline.txt"

bench-save: ## Run benchmarks and save results with timestamp
	@echo "Running benchmarks..."
	@mkdir -p benchmarks
	@TIMESTAMP=$$(date +%Y%m%d_%H%M%S); \
	go test -bench=. -benchmem -count=5 -run=^$$ ./... | tee benchmarks/bench_$$TIMESTAMP.txt
	@echo "Results saved to benchmarks/bench_$$TIMESTAMP.txt"

bench-compare: ## Compare latest benchmark with baseline (requires benchstat)
	@if [ ! -f benchmarks/baseline.txt ]; then \
		echo "Error: No baseline found. Run 'make bench-baseline' first."; \
		exit 1; \
	fi
	@LATEST=$$(ls -t benchmarks/bench_*.txt 2>/dev/null | head -1); \
	if [ -z "$$LATEST" ]; then \
		echo "Error: No benchmark results found. Run 'make bench-save' first."; \
		exit 1; \
	fi; \
	echo "Comparing baseline with $$LATEST"; \
	benchstat benchmarks/baseline.txt $$LATEST

# Profiling
profile-cpu: ## Run CPU profiling
	@mkdir -p profiles
	go test -bench=BenchmarkRoundTripMedium -cpuprofile=profiles/cpu.prof -run=^$$ ./...
	@echo "CPU profile saved to profiles/cpu.prof"
	@echo "View with: go tool pprof -http=:8080 profiles/cpu.prof"

profile-mem: ## Run memory profiling
	@mkdir -p profiles
	go test -bench=BenchmarkRoundTripMedium -memprofile=profiles/mem.prof -run=^$$ ./...
	@echo "Memory profile saved to profiles/mem.prof"
	@echo "View with: go tool pprof -http=:8080 profiles/mem.prof"

profile-all: ## Run both CPU and memory profiling
	@mkdir -p profiles
	go test -bench=BenchmarkRoundTripMedium -cpuprofile=profiles/cpu.prof -memprofile=profiles/mem.prof -run=^$$ ./...
	@echo "Profiles saved to profiles/"
	@echo "View CPU: go tool pprof -http=:8080 profiles/cpu.prof"
	@echo "View MEM: go tool pprof -http=:8081 profiles/mem.prof"

# Analysis
escape: ## Run escape analysis
	cd .. && go build -gcflags="-m -m" ./... 2>&1 | grep "escapes to heap"

fieldalignment: ## Check struct field alignment
	@which fieldalignment > /dev/null || (echo "Installing fieldalignment..." && go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest)
	cd .. && fieldalignment ./...

staticcheck: ## Run static analysis
	@which staticcheck > /dev/null || (echo "Installing staticcheck..." && go install honnef.co/go/tools/cmd/staticcheck@latest)
	cd .. && staticcheck ./...

vet: ## Run go vet
	cd .. && go vet ./...

# Build
build: ## Build the project
	cd .. && go build ./...

build-release: ## Build optimized release binary
	cd .. && go build -ldflags="-s -w" -trimpath -o bin/psrp-client ./cmd/psrp-client

build-pgo: ## Build with profile-guided optimization (requires profiles/default.pprof)
	@if [ ! -f profiles/default.pprof ]; then \
		echo "Error: No PGO profile found. Run 'make profile-cpu' first and copy profiles/cpu.prof to profiles/default.pprof"; \
		exit 1; \
	fi
	cd .. && go build -pgo=test/profiles/default.pprof -o bin/psrp-client ./cmd/psrp-client

# Cleanup
clean: ## Clean build artifacts and benchmark results
	rm -rf ../bin/
	rm -rf benchmarks/
	rm -rf profiles/
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
