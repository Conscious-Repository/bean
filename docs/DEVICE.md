# What a device signs, and how it hands it over

This is for the people who build an instrument and its firmware. It says what the device must write so that bean, and anyone else, can check it. No device exists yet. The format is fixed first, and the hardware is built to meet it.

It is a draft at version 0. It changes when a real device shows that it must.

## The idea

- The device has a key that it made and that never leaves it.
- For each acquisition, the device writes one statement and signs it. The statement names the raw data by its SHA-256, and lists the settings.
- The device hands over a folder: the statements, the raw data, and its own time.
- The person brings the folder into their log, and says that the captures are about them. The device does not know whose body it measured, so it does not say.

A statement is not a bean record. It has no subject. The person's own record points at it. So one device can serve many people, and a signature never says more than its signer can know.

## What a signature supports

A device's signature shows that this key signed these bytes, with these settings, in this order. With a certificate from the factory, it shows that the key belongs to a device of a named model. It does not show that the reading is accurate, or whose body was measured.

## Try it without a device

```
go run ./tools/device/sim -out /tmp/dev -n 3 -cert     # a software device writes a folder
python3 tools/device/check.py /tmp/dev                 # the check, in one Python file
make device                                            # both checks, on folders good and wrong
```

`tools/device/check.py` needs Python 3 and the `cryptography` package, and no code of bean. Run it on what your firmware writes. It is the check that bean will apply: `make device` holds it and bean's own check to one verdict.

## The folder

| Entry | What it is |
|---|---|
| `device.json` | One object: which device, its time now, and its certificate if it has one. |
| `captures.jsonl` | One statement a line, oldest first. |
| `raw/<sha256>` | Each raw file, named by the SHA-256 of its bytes, in lower-case hex. |

The folder is the same over a cable, Bluetooth or Wi-Fi. The link is not trusted for integrity: the statements are signed. The link must still be private, and its pairing key is not the signing key.

The device keeps an acquisition until the host confirms that it holds it. A later hand-over starts where the last one ended.

## How every object is written

Each object is JSON in one fixed form, so that every processor writes the same bytes.

1. The keys of an object are in byte order. For the names below, that is alphabetical order.
2. There is no space and no line break inside an object.
3. A value is text or a whole number from 0 to 9007199254740991. There is no fraction, no negative number, no `true`, `false` or `null`.
4. Text is printable ASCII, from space to `~`, without `"` and without `\`. So no text needs an escape.

Write a fraction as a whole number in a smaller unit: `depth_mm`, not `depth_cm`. A firmware can fill a fixed template.

## A statement

```
{"bean":"capture/0","calibration":"phantom-7 2026-11-01","device":{"fw":"0.1.0","hw":"A","key":"p256:02e0...","model":"sim-1"},"kind":"sim.noise","prev":"","raw":[{"bytes":4096,"name":"samples.bin","sha256":"3129..."}],"selftest":"pass","seq":{"boot":1,"capture":1},"settings":{"depth_mm":40,"gain_db":12,"probe":"linear-5MHz"},"sig":"p256:ad97...","time":"2026-11-11T17:01:00Z"}
```

| Member | Value |
|---|---|
| `bean` | `capture/0`. |
| `calibration` | Optional. The calibration in force, as text. |
| `device.key` | The device's public key (below). |
| `device.model`, `device.hw`, `device.fw` | The model, the hardware revision and the firmware version. |
| `kind` | What was acquired, as a name in lower case with dots: `ultrasound.bmode`. |
| `prev` | The id of the statement before this one. Empty for capture 1. |
| `raw` | One entry or more. Each has `name`, `sha256` (64 lower-case hex digits) and `bytes`. `name` is a label and never a path: a reader finds the file by its SHA-256. |
| `selftest` | `pass`, `fail` or `none`. |
| `seq.capture` | The count of acquisitions since manufacture. It starts at 1 and goes up by one. |
| `seq.boot` | The count of starts since manufacture. It never goes back. |
| `settings` | Every setting of the acquisition, as text or whole numbers. One level deep. |
| `sig` | The signature (below). |
| `time` | The device's time in UTC, as `2026-11-11T17:01:00Z`. Empty if the device has no clock. |

A statement has these members and no other. `fw`, `hw`, `model`, a raw `name`, and `calibration` when it is there, are not empty. `kind` uses lower-case letters, digits, `_`, `-` and dots, and starts with a letter or a digit. `model` and `hw` are the same in every statement of a folder.

The device's time is a claim. The counters give the order. `prev` ties each statement to the one before it, so a statement cannot be taken out of the middle or put in.

## The key and the signature

The key is P-256 or Ed25519, written as bean writes a key (SPEC.md 4.4).

- P-256: `p256:` and the compressed point, 33 bytes as 66 hex digits.
- Ed25519: `ed25519:` and the 32 bytes as 64 hex digits.

Use P-256 if the device has a secure element: most of them sign with it.

The signature is the signature of a bean record (SPEC.md 4.5):

1. Take the statement without its `sig` member, in the form above. Compute its SHA-256. The result, 32 bytes, is the statement's id. `prev` of the next statement is `sha256:` and these bytes in hex.
2. P-256: sign the 32 bytes with ECDSA and SHA-256. A secure element that signs a digest gets the SHA-256 of the 32 bytes. Write r and s as 32 bytes each, 128 hex digits, after `p256:`. If s is above half the order of the curve, replace it by the order minus s.
3. Ed25519: sign the 32 bytes. Write the 64 bytes as 128 hex digits after `ed25519:`.

## device.json

```
{"bean":"device/0","cert":{...},"key":"p256:02e0...","time":"2026-11-11T18:00:00Z"}
```

`key` is the device's key. Every statement in the folder is signed by it. `time` is the device's clock at the hand-over, or empty. The host sets it against its own clock, so that a wrong device clock can be corrected later. `cert` is optional.

## The certificate from the factory

```
{"bean":"device-cert/0","hw":"A","issuer":"p256:0291...","key":"p256:02e0...","model":"sim-1","sig":"p256:96d4..."}
```

The manufacturer signs each device's key, model and hardware revision at the factory, with its own key. The signature is made the same way as a statement's, by `issuer`. The certificate's `model` and `hw` must be the ones in every statement. A reader decides whether they trust the issuer: the manufacturer publishes its key. A check prints the issuer's key and says so. A certificate from an unknown key shows nothing.

## Rules for the firmware

- **One line, one statement.** `captures.jsonl` ends each statement with one line feed (LF). It has no blank line and no carriage return. `device.json` is one object with at most one line feed after it.
- **Nothing else in the folder.** The folder holds `device.json`, `captures.jsonl` and `raw`, and `raw` holds only files that a statement names. A check refuses anything more.
- **A number is used once, and only for a statement that exists.** `seq.capture` goes up by one for each statement. If an acquisition fails, write a statement for it with `selftest` `fail`, or do not count it. A folder is a run with no gap: one lost statement makes every later hand-over that spans it fail.
- **Keep the id of the last statement.** The device needs it for `prev` of the next one. It must survive a loss of power, with the two counters.
- **A later hand-over starts in the middle.** The first statement of a folder has a `prev` that a check cannot follow: the statement before it was handed over earlier. A check ties each later line to the one before it.
- **What a carrier can do.** A carrier cannot take a statement out of the middle of a run. A carrier can drop statements from the start or the end of a folder. A carrier can also change `device.json`, which is not signed: its time, or whether a certificate is there.

## What the hardware needs

- A place for the key that the firmware cannot read out: a secure element, or the secure key store of the processor.
- A step on the production line that reads the public key and issues the certificate.
- Signed firmware. A device key signs whatever the firmware hands it.
- Two counters, and the id of the last statement, that survive a loss of power.
- Enough storage and time to hash each raw acquisition as it is written.

## What a check refuses

`make device` tries each of these, and both checks refuse it:

- a raw file that is changed, missing, or named by no statement;
- a setting changed after signing, or a signature by another key;
- a statement taken out of the middle, or two in the wrong order;
- a fraction, an escape, an accent, `true` or `null`, a nested setting, or a member that the format does not have;
- keys that are not in order, or a space inside a line;
- a signature whose s is in the upper half;
- a certificate for another key or another model, or one that its issuer did not sign;
- an Ed25519 key or signature that is not a real point, such as the key that makes every signature pass;
- a time with a fraction, a key in upper-case hex, or a file in the folder that is not part of it.

## Not decided

- How the person's record points at a statement, and what bean shows for it. `bean pair` and `bean import-device` are not built ([ROADMAP.md](ROADMAP.md), path 3).
- Whether a device may report a value of its own, beside raw data. Today it may not: a value comes from raw data, by a method that anyone can run again.
- How a key or a model is withdrawn after a fault.
- How the host confirms that it holds an acquisition, so that the device may delete it.
- The largest raw file, and whether raw data may stay outside the log.
