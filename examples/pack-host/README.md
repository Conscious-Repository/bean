# A lab hosts two public packs

One static file server serves the two NHANES packs under their SHA-256 names.
The lab writes files locally. A reader needs a host address and the expected digest from [REFERENCE.md](../../docs/REFERENCE.md#how-a-pack-travels).
Each lab can use its own software and domain. This example uses Caddy and local disk.

## Rehearse on one machine

From the repository root, use Bash, Go, curl, shasum, Python 3 and Caddy.
Start with an empty work directory outside the checkout. Keep that shell open.

```sh
recipe="$PWD/examples/pack-host"
work=$(mktemp -d)
mkdir "$work/cdc"
(
  set -eu
  for f in DEMO_J TCHOL_J HDL_J GHB_J CBC_J BMX_J BIOPRO_J FERTIN_J; do
    curl -fLsS --retry 2 -o "$work/cdc/$f.XPT" "https://wwwn.cdc.gov/Nchs/Data/Nhanes/Public/2017/DataFiles/$f.xpt"
  done
  bash "$recipe/prepare.sh" "$work/cdc" "$work/packs"
)
```

Use new directories for the build and the reader. The builder checks the survey files. The script checks both finished packs before moving either to its digest name.
Use a new output directory on each build. After both checks pass, run:

```sh
(
  cd "$work/packs" || exit 1
  export XDG_DATA_HOME="$work/state" XDG_CONFIG_HOME="$work/config"
  exec caddy run --config "$recipe/Caddyfile"
)
```

This listens on loopback only. It serves GET and HEAD for two exact paths. Other paths and methods return 404.
Run it from the directory of ordinary pack files. Keep control of that directory with the operator.
In a second terminal at the repository root, run the reader check below with the loopback origin.
While the host is closed, check the two requests below. Both must print 404 for the example configuration.

```sh
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/
curl -sS -o /dev/null -w '%{http_code}\n' -X PUT --data 'trial' http://127.0.0.1:8080/upload
```

Stop Caddy with Ctrl-C. Restart it from the same directory and repeat the reader check.
For each later cleanup run, repeat the check to confirm it kept both packs.

## Check either lab from a reader's machine

Use the lab's HTTPS origin for a live host. Use the two digests from a trusted copy of REFERENCE.md.
The checker makes anonymous HEAD and GET requests to that origin. Each must return 200 directly.
It hashes the body before writing any pack bytes to standard output. Errors give a failing exit status.

```sh
origin=http://127.0.0.1:8080
small=10555d6769421d4305cbcd4ff14960d8b966ddd829a50581a9584a60a84d29e2
large=5cae07db64295d565ffae7c251c17fdb9f043dc0638868557275f8b0b0311efa
reader=$(mktemp -d)
(
  set -eu
  for digest in "$small" "$large"; do
    python3 -I examples/pack-host/check.py "$origin" "$digest" > "$reader/$digest"
  done
)
```

Continue only if both checks pass. To test a second lab, have its operator rebuild the packs or copy the verified bytes.
It serves them from its own disk and domain. Take the first origin offline for the rehearsal, or block it on the reader's machine.
Set `origin` to the second URL and use a fresh reader directory. Run the same check from outside that lab. The digests come from the same independent copy of REFERENCE.md.
This tests the two reads at that time. Logging, isolation and retention need checks by each operator.

With both hosts out of reach, try the same made-up reading in a fresh log.
Run these commands in the reader's shell with the checked pack and a current `bean` on PATH.

```sh
(
  set -eu
  export BEAN_DIR="$reader/trial-log" TZ=UTC
  mkdir "$BEAN_DIR"
  bean init >/dev/null 2>&1
  bean add lab.cholesterol-total 187 -unit mg/dL -at 2026-11-01
  bean compare -pack "$reader/$large" -cohort male-30-39 -type lab.cholesterol-total
)
```

For the 20-measure pack the percentile is 46 and N is 339. Each reader can discard this trial log.

## From loopback to the lab

Follow the static-host stages in [HOST-PLAN.md](../../docs/HOST-PLAN.md#the-read-only-host).
Choose the isolated machine or network, operator and domain. Use a patched server build.
The initial loopback test used Caddy 2.11.9. Its upstream macOS binary contained symbols flagged by govulncheck, including [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859).
The smallest lab setup can use Caddy itself for TLS.
In a local copy of the Caddyfile, set the site address to the private HTTPS name and port.
Change `bind` to the isolated interface. Add `tls /path/to/lab-cert.pem /path/to/lab-key.pem` inside the site block.
Use the lab's own certificate authority for the closed trial. Give the reader its public CA certificate through an independent channel.
Set `SSL_CERT_FILE` to that CA file when running the checker. A trusted public certificate uses the reader's normal trust store.
Keep the two-path allowlist and the service account's read-only access to the packs.
Check logs at the TLS endpoint and file host for the address and retention rules in the plan.
Each owner decides when to open their endpoint. A second host of the same bytes establishes availability.
A lab publishing a new data source also needs a way for readers to judge its source and method.
