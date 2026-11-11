# make check runs every check that a commit needs. make fuzz and make smoke
# run the longer checks. Tools: staticcheck and govulncheck on the PATH,
# and a python3 with tools/requirements.txt installed for the vectors.
PY ?= python3
FUZZTIME ?= 2m

.PHONY: help check build fmt vet test lint vuln prose vectors second compare bundles page pagetest example differential demo fuzz smoke

help:
	@echo "make check         every check a commit needs (fmt vet build test lint vuln prose vectors second compare bundles pagetest example differential demo)"
	@echo "make test          go test -race -failfast ./..."
	@echo "make prose         no sentence of a document is longer than 25 words"
	@echo "make vectors       regenerate vectors/ and check them byte for byte and with Python"
	@echo "make second        compare bean with the second verifier and the conformance folders"
	@echo "make compare       compute every saved comparison again with Python and pandas"
	@echo "make bundles       check share bundles, good and damaged, with bean, with Python and with the page"
	@echo "make page          build bean-check.html, the page that checks a bundle in a browser"
	@echo "make pagetest      try that page in a real browser (needs node with Playwright)"
	@echo "make example       run examples/blood offline, on synthetic reference files"
	@echo "make differential  compare this build with OLD (default origin/main)"
	@echo "make fuzz          run each fuzz target for FUZZTIME (default 2m)"
	@echo "make smoke         run the end-to-end and adversarial suite, with timings on a large log"

check: fmt vet build test lint vuln prose vectors second compare bundles pagetest example differential demo

build:
	go build ./...

fmt:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...
	GOOS=js GOARCH=wasm go vet ./tools/page/check

test:
	# Do not time the scale test under race instrumentation.
	go test -race -failfast -skip TestVerifyScales ./...
	go test -failfast ./core -run TestVerifyScales

lint:
	staticcheck ./...

vuln:
	govulncheck ./...

# The documents use Simplified Technical English. This checks the rule of
# it that can be counted: a sentence has at most 25 words. DECISIONS.md is a
# dated log and is not rewritten.
DOCS = README.md CORE.md SPEC.md SECURITY.md UPGRADING.md CONTRIBUTING.md \
	docs/COMMANDS.md docs/ONBOARDING.md docs/SHARE.md docs/COMPARE.md \
	docs/IMPORTERS.md docs/ROADMAP.md docs/EXTENSIONS.md \
	docs/PROVIDER-INGESTION.md docs/GOVERNANCE-PROPOSAL.md \
	cmd/bean/vocabulary.md cmd/bean-import-lab/README.md \
	cmd/bean-import-apple-health/README.md cmd/bean-import-nhanes/README.md \
	examples/blood/README.md tools/second/README.md
prose:
	$(PY) -I tools/checkprose.py $(DOCS)

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

# An independent implementation in Python must compute every number of every
# comparison that bean saves on the synthetic NHANES files. With
# BEAN_NHANES_DIR set to a folder of CDC's own files, on those too.
compare:
	PY=$(PY) bash tools/compare/run.sh

# bean check-bundle and the Python checker, which follows docs/SHARE.md, must
# give the same verdict on every bundle, good and damaged. The page that
# checks a bundle in a browser is built and must print what bean prints.
# This needs node, to run the page's check without a browser.
bundles:
	PY=$(PY) bash tools/second/bundles.sh

# One file that checks a bundle in a browser, with no install and no
# network: the check of bean check-bundle, built for a browser. The same
# commit and the same Go give the same bytes; it prints their SHA-256.
page:
	go run ./tools/page -o bean-check.html

# The page's own script, buttons and content policy, in a real browser
# opened from the disk. CI has no browser to drive, so this runs where
# Playwright is.
pagetest:
	bash tools/page/smoke.sh

# The quickstart must run from a fresh clone and give the numbers it was
# written with. Here it runs offline on the synthetic reference files;
# examples/blood/run.sh without --synthetic fetches CDC's.
example:
	bash examples/blood/run.sh --synthetic >/dev/null

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
	go test ./cmd/bean-import-lab -run '^$$' -fuzz '^FuzzLab$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./cmd/bean-import-apple-health -run '^$$' -fuzz '^FuzzAppleExport$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s
	go test ./internal/xpt -run '^$$' -fuzz '^FuzzParse$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 5s

smoke:
	tools/smoke.sh
