// smoke.cjs: opens a built bean-check.html in a real browser, from the
// disk as a reader does, and uses its buttons. tools/page/smoke.sh makes
// the bundles and runs it. It needs Playwright with Chromium.
//
//     node smoke.cjs <bean-check.html> <folder with good/, good.zip, bad/ and sealed.bean> <six words>
const { chromium } = require("playwright");
const path = require("path");

(async () => {
  const [pagePath, dir] = process.argv.slice(2, 4).map((p) => path.resolve(p));
  const words = process.argv[4] || "";
  const browser = await chromium.launch();
  const page = await (await browser.newContext()).newPage();
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
  expect("a good zip file passes", (await title()) === "This bundle passed the check." && (await page.locator("#rows tr").count()) > 0);
  expect("what is not part of the bundle is named in the verdict", (await page.textContent("#verdict-text")).includes("README.txt"));

  await page.setInputFiles("#folder", path.join(dir, "good"));
  await settled();
  expect("a good folder passes", (await title()) === "This bundle passed the check." && (await page.locator("#rows tr").count()) > 0);

  // A damaged bundle after a good one: no row of the good one stays.
  await page.setInputFiles("#folder", path.join(dir, "bad"));
  await settled();
  expect("a damaged folder fails", (await title()) === "This bundle FAILED the check.");
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
  expect("other words open nothing", (await title()) === "Nothing was checked." && !(await rowsShown()) && (await page.locator("#words-box").isVisible()));
  await page.fill("#words", "  " + words.toUpperCase() + " ");
  await page.click("#open");
  await settled();
  expect("the six words open the package", (await title()) === "This bundle passed the check." && (await page.locator("#rows tr").count()) > 0 && !(await page.locator("#words-box").isVisible()));

  // The page loaded nothing but itself, and a request from inside it fails.
  const reach = await page.evaluate(() => fetch("https://example.com/").then(() => "reached", () => "blocked"));
  expect("the page cannot fetch", reach === "blocked");
  expect("the page asked for nothing but itself", requests.every((u) => u === "file://" + pagePath));
  expect("no script error", errors.length === 0);
  if (errors.length) console.log(errors.join("\n"));

  await browser.close();
  if (failed) process.exit(1);
  console.log("page: a real browser opens it from the disk, passes a good bundle as a folder and as a zip file, opens a sealed package with its words only, fails a damaged one, keeps no earlier verdict, and loads nothing");
})().catch((err) => { console.error(err); process.exit(1); });
