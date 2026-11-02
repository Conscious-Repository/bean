# make check runs every check that a commit needs. make fuzz and make smoke
# run the longer checks. Tools: staticcheck and govulncheck on the PATH,
# and a python3 with tools/requirements.txt installed for the vectors.
PY ?= python3
FUZZTIME ?= 2m

.PHONY: help check build fmt vet test lint vuln vectors second differential demo fuzz smoke

help:
	@echo "make check         every check a commit needs (fmt vet build test lint vuln vectors second differential demo)"
	@echo "make test          go test -race -failfast ./..."
	@echo "make vectors       regenerate vectors/ and check them byte for byte and with Python"
	@echo "make second        compare bean with the second verifier and the conformance folders"
	@echo "make differential  compare this build with OLD (default origin/main)"
	@echo "make fuzz          run each fuzz target for FUZZTIME (default 2m)"
	@echo "make smoke         build a large log and time the commands"

check: fmt vet build test lint vuln vectors second differential demo apple-health

build:
	go build ./...

fmt:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

test:
	# Do not time the scale test under race instrumentation.
	go test -race -failfast -skip TestVerifyScales ./...
	go test -failfast ./core -run TestVerifyScales

lint:
	staticcheck ./...

vuln:
	govulncheck ./...

# The generator must reproduce every committed vector byte for byte.
vectors:
	go run ./tools/genvectors >/dev/null
	git diff --exit-code -- vectors
	@test -z "$$(git status --porcelain -- vectors)" || { git status --porcelain -- vectors; exit 1; }
	$(PY) -I tools/checkvectors.py >/dev/null
	$(PY) -I tools/checksecp256k1.py >/dev/null

# A second verifier, written from CORE.md and SPEC.md only, must print the
# same verify lines and exit codes as bean on every scenario log.
second:
	PY=$(PY) bash tools/second/run.sh

# The build of this tree must print what the build of OLD (default
# origin/main) prints on every corpus log. Set ALLOW only for a difference
# that the change means to make, and say so in the commit.
OLD ?= origin/main
differential:
	PY=$(PY) bash tools/differential/run.sh $(OLD)

demo:
	./demo.sh >/dev/null

fuzz:
	go test ./core -run '^$$' -fuzz '^FuzzCanonical$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./core -run '^$$' -fuzz '^FuzzCheckFile$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./core -run '^$$' -fuzz '^FuzzVerifyFiles$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./core -run '^$$' -fuzz '^FuzzSealValue$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./core -run '^$$' -fuzz '^FuzzSealSets$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./core -run '^$$' -fuzz '^FuzzTooNewGate$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./cmd/bean-import-csv -run '^$$' -fuzz '^FuzzConvert$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./cmd/bean-import-labcorp -run '^$$' -fuzz '^FuzzLabcorp$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s

smoke:
	tools/smoke.sh

.PHONY: apple-health
apple-health:
	PY=$(PY) bash tools/apple-health/run.sh
