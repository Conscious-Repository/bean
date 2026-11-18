// smoke.cjs: opens a built bean-check.html in a real browser, from the
// disk as a reader does, and uses its buttons. tools/page/smoke.sh makes
// the bundles and runs it. It needs Playwright with Chromium.
//
//     node smoke.cjs <bean-check.html> <folder with good/, good.zip, bad/, sealed.bean, forkey.bean and two.bean> <six words>
const { chromium } = require("playwright");
const path = require("path");

(async () => {
  const [pagePath, dir] = process.argv.slice(2, 4).map((p) => path.resolve(p));
  const words = process.argv[4] || "";
  const browser = await chromium.launch();
  const page = await (await browser.newContext({ acceptDownloads: true })).newPage();
  const requests = [], errors = [];
  page.on("request", (r) => requests.push(r.url()));
  page.on("pageerror", (e) => errors.push(String(e)));
  let failed = 0;
  const expect = (what, ok) => { if (!ok) { failed++; console.log("FAIL: " + what); } };
  const title = () => page.textContent("#verdict-title");
  const rowsShown = () => page.locator("#rows-box").isVisible();
  const settled = () => page.waitForFunction(() => !document.getElementById("pick-zip").disabled);

  await page.goto("file://" + pagePath);
  await settled();

  await page.setInputFiles("#zip", path.join(dir, "good.zip"));
  await settled();
  expect("a good zip file passes", (await title()).startsWith("These records passed the check.") && (await page.locator("#rows tr").count()) > 0);
  expect("what is not part of the bundle is named in the details", (await page.textContent("#head")).includes("not read: README.txt"));
  expect("the signer's identity words are shown", /^([a-z]+ ){7}[a-z]+$/.test(await page.textContent("#who")) && (await page.locator("#who-box").isVisible()));

  await page.setInputFiles("#folder", path.join(dir, "good"));
  await settled();
  expect("a good folder passes", (await title()).startsWith("These records passed the check.") && (await page.locator("#rows tr").count()) > 0);

  // A damaged bundle after a good one: no row of the good one stays.
  await page.setInputFiles("#folder", path.join(dir, "bad"));
  await settled();
  expect("a damaged folder fails", (await title()) === "These records FAILED the check.");
  expect("a failed bundle does not show who signed", !(await page.locator("#who-box").isVisible()) && !(await page.textContent("#head")).includes("signed by identity"));
  expect("a failed bundle shows no record", !(await rowsShown()) && (await page.locator("#rows tr").count()) === 0);

  // A good one again, then a zip file that is none: the pass does not stay.
  await page.setInputFiles("#zip", path.join(dir, "good.zip"));
  await settled();
  await page.setInputFiles("#zip", { name: "notes‮gnp.zip", mimeType: "application/zip", buffer: Buffer.from("not a zip file") });
  await settled();
  expect("what is no bundle is called not checked", (await title()) === "Nothing was checked." && !(await rowsShown()));
  expect("a name cannot reorder the page's own words", !(await page.textContent("#verdict-text")).includes("‮"));

  // While a check runs, the last verdict is off the page.
  await page.setInputFiles("#zip", path.join(dir, "good.zip"));
  await settled();
  const during = await page.evaluate(async () => {
    const input = document.getElementById("zip");
    const data = new DataTransfer();
    data.items.add(new File(["not a zip file"], "x.zip"));
    input.files = data.files;
    input.dispatchEvent(new Event("change"));
    await new Promise((next) => setTimeout(next, 5));
    return document.getElementById("result").hidden;
  });
  await settled();
  expect("the last verdict is taken off before the next check", during === true);

  // A sealed package: the page asks for its words, shows nothing until it
  // has them, keeps asking after wrong ones, and opens with the right ones.
  await page.setInputFiles("#zip", path.join(dir, "sealed.bean"));
  await settled();
  expect("a sealed package asks for its words and shows nothing", (await page.locator("#words-box").isVisible()) && !(await page.locator("#result").isVisible()));
  await page.fill("#words", "abandon ability able about above absent");
  await page.click("#open");
  await settled();
  expect("other words open nothing", (await title()) === "This file did not open." && !(await rowsShown()) && (await page.locator("#words-box").isVisible()));
  await page.fill("#words", "  " + words.toUpperCase() + " ");
  await page.click("#open");
  await settled();
  expect("the six words open the package", (await title()).startsWith("These records passed the check.") && (await page.locator("#rows tr").count()) > 0 && !(await page.locator("#words-box").isVisible()));
  expect("no typed word stays in the page", (await page.inputValue("#words")) === "");

  // An opened package says which share it is, and that the words do not
  // show who sealed it. Rows with nothing to remark leave the box green.
  expect("the page says which share this is", /its id starts [0-9a-f]{12}\./.test(await page.textContent("#share-line")));
  expect("the page does not say that the words show who sealed it", (await page.textContent("#opened")).includes("They do not show who sealed it"));
  expect("a pass with no marked row is green", (await page.getAttribute("#verdict", "class")) === "verdict ok");
  expect("a comparison's numbers are labelled as claimed", (await page.locator("#rows td.text", { hasText: "As claimed, not computed here: percentile" }).count()) > 0);

  // A file that the package carries can be saved, and is the file that was
  // checked: its SHA-256 starts the name the page gives it.
  expect("the carried files are listed", (await page.locator("#files button").count()) > 0);
  const [download] = await Promise.all([page.waitForEvent("download"), page.locator("#files button.quiet").first().click()]);
  const saved = require("fs").readFileSync(await download.path());
  const digest = require("crypto").createHash("sha256").update(saved).digest("hex");
  expect("a saved file is the file that was checked", download.suggestedFilename().includes("-" + digest.slice(0, 8) + "."));

  // A text file can be read in the page, as text.
  const readers = page.locator("#files button:not(.quiet)");
  if (await readers.count()) {
    await readers.first().click();
    expect("a text file is shown in the page", (await page.locator("#reader-box").isVisible()) && (await page.textContent("#reader")).length > 0);
  }

  // A package that is sealed to a key. The page asks its check what the
  // file asks for before it shows anything. It shows no box for words, and
  // says that the file is sealed to a key and that it cannot open it.
  await page.setInputFiles("#zip", path.join(dir, "forkey.bean"));
  await settled();
  expect("a package that is sealed to a key gets no box for words", !(await page.locator("#words-box").isVisible()) && (await page.locator("#result").isVisible()));
  const toKey = await page.textContent("#verdict-text");
  expect("the page says that the file is sealed to a key", (await title()) === "This file is sealed to a key.");
  expect("the page says that it cannot open it, and what opens it", toKey.includes("This page cannot open") && toKey.includes("with bean or with the age tool") && toKey.includes("a file that is sealed with six words"));
  expect("nothing of a package that is sealed to a key is shown", !(await rowsShown()) && !(await page.locator("#who-box").isVisible()) && !(await page.locator("#problems-box").isVisible()) && !(await page.locator("#opened").isVisible()));
  expect("a file that did not open is not green", (await page.getAttribute("#verdict", "class")) === "verdict bad");

  // An age file that names two readers: no box for words, and the reason.
  await page.setInputFiles("#zip", path.join(dir, "two.bean"));
  await settled();
  expect("a file for two readers gets no box for words", !(await page.locator("#words-box").isVisible()) && (await title()) === "This file did not open.");
  expect("the page says why", (await page.textContent("#problems")).includes("has 2 stanzas") && !(await page.textContent("#verdict-text")).includes("six words"));

  // After those, a package with words still gets its box.
  await page.setInputFiles("#zip", path.join(dir, "sealed.bean"));
  await settled();
  expect("a package with words gets the box again", (await page.locator("#words-box").isVisible()) && !(await page.locator("#result").isVisible()));

  // A second file with the same name: the box for words of the first does
  // not stay for a file that asks for no words.
  await page.setInputFiles("#zip", path.join(dir, "same", "sealed.bean"));
  await settled();
  expect("a file of the same name that is sealed to a key gets no box for words", !(await page.locator("#words-box").isVisible()) && (await title()) === "This file is sealed to a key.");
  await page.setInputFiles("#zip", path.join(dir, "sealed.bean"));
  await settled();
  await page.setInputFiles("#zip", path.join(dir, "good.zip"));
  await settled();
  expect("a file that needs no words takes the box away", !(await page.locator("#words-box").isVisible()) && (await title()).startsWith("These records passed the check."));

  // A row that the check marks turns the box amber, counts the rows to
  // read, and draws the mark apart from the record's own text.
  await page.setInputFiles("#zip", path.join(dir, "marked.zip"));
  await settled();
  expect("a pass with a marked row is amber and says so", (await page.getAttribute("#verdict", "class")) === "verdict warn" && (await title()).includes("Read the 1 row that is marked below."));
  expect("the mark is drawn as the check's own", (await page.textContent("#rows .mark")) === "names NO reference data");

  // The page loaded nothing but itself, and a request from inside it fails.
  const reach = await page.evaluate(() => fetch("https://example.com/").then(() => "reached", () => "blocked"));
  expect("the page cannot fetch", reach === "blocked");
  expect("the page asked for nothing but itself", requests.every((u) => u === "file://" + pagePath));
  expect("no script error", errors.length === 0);
  if (errors.length) console.log(errors.join("\n"));

  await browser.close();
  if (failed) process.exit(1);
  console.log("page: a real browser opens it from the disk, passes a good bundle as a folder and as a zip file, opens a sealed package with its words only and saves a file it carries, says of a package that is sealed to a key that it cannot open it, fails a damaged one, keeps no earlier verdict, and loads nothing");
})().catch((err) => { console.error(err); process.exit(1); });
