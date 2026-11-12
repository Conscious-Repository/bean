# The whole loop, on a synthetic report

```
examples/blood/run.sh
```

One script, in a throwaway folder. It prints each command's own output, so what you see is what the commands say. Nothing here is a real person's data: `report.txt` is made up, in the layout of a Labcorp report, with values a person could have.

| Step | Command | What it shows |
|---|---|---|
| 1 | `bean init`, `bean import lab report.txt` | The report is kept first, encrypted, then its 16 readings are signed. |
| 2 | `curl`, `bean import nhanes <file>` | Six public files from the US CDC (about 7 MB) are kept, each pinned by its SHA-256. bean itself never goes online. |
| 3 | `bean compare -sex male -age 30-39 -type 'lab.*' -save` | Where 12 of the readings lie among men of 30 to 39, and then among those with a BMI of 20 to 27. Each row shows how many people it rests on. |
| 4 | `bean share -out bundle -type 'comparison.*'` | The saved comparisons, their readings, the records of the reference files and the identity records, as a folder. |
| 5 | `bean check-bundle`, `checkbundle.py`, `reproduce.py` | The bundle is checked, and every comparison computed again in Python, with no bean. |
| 6 | `bean share -seal -out for-a-reader.bean`, `bean check-bundle`, `unseal.py` | The same records as one encrypted file that opens with six words. It is opened with the words, by bean and by Python, and not with other words. |
| 7 | `bean reply`, `bean accept` | The reader, with a log of their own, signs a note about one reading and seals it. You see who signed it and take the note into your log under a name. |

## Be the reader, in a browser

The script ends with the path of the sealed package and its six words. To open it as a reader with no terminal would:

```
make page
```

Open `bean-check.html` in a browser. Press "Choose the file you received", pick the package, type the six words, and press Open. The page checks the package on your machine and shows what it holds. It sends nothing. It shows the signer's eight identity words: the script prints the same eight after "Your identity words".

Try it with one word changed: the file does not open. The plain folder, `bundle`, opens with "Or choose a folder" and needs no words.

## What the script checks

At the end it compares its numbers with [`expected.txt`](expected.txt) and fails if one differs. Those numbers are statistics of CDC's public files for the made-up readings.

- `run.sh <folder>` keeps everything in a folder you name: the log, the files, the bundle and the package.
- `BEAN_NHANES_DIR=<folder>` uses CDC files you already have.
- `run.sh --synthetic` uses the small made-up reference files in `internal/nhanes/testdata`, offline. Its numbers ([`expected-synthetic.txt`](expected-synthetic.txt)) describe nobody; CI runs it so that this example cannot rot.
- Step 5's Python checks need `pip install -r tools/requirements.txt`, and are skipped without it.

A comparison says where a value lies in a sample. It does not say whether the value is healthy. [docs/COMPARE.md](../../docs/COMPARE.md) and [docs/SHARE.md](../../docs/SHARE.md) have the limits.

To do the same with your own report, follow [docs/ONBOARDING.md](../../docs/ONBOARDING.md). Your identity then has 24 recovery words that you write on paper, and you review what an import read before it saves.
