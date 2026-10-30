# make check runs every check that a commit needs. make fuzz and make smoke
# run the longer checks. Tools: staticcheck and govulncheck on the PATH,
# and a python3 with the cryptography package (AES-SIV) for the vectors.
PY ?= python3
FUZZTIME ?= 2m

.PHONY: check build fmt vet test race lint vuln vectors demo fuzz smoke

check: fmt vet build race lint vuln vectors demo

build:
	go build ./...

fmt:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

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

demo:
	./demo.sh >/dev/null

fuzz:
	go test ./core -run '^$$' -fuzz '^FuzzCanonical$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./core -run '^$$' -fuzz '^FuzzCheckFile$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./core -run '^$$' -fuzz '^FuzzVerifyFiles$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./cmd/bean-import-csv -run '^$$' -fuzz '^FuzzConvert$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./cmd/bean-import-labcorp -run '^$$' -fuzz '^FuzzLabcorp$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s

smoke:
	tools/smoke.sh
