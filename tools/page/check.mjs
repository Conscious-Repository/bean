// check.mjs: runs the check that a built bean-check.html carries, without
// a browser, so that scripts can compare it with bean check-bundle.
//
//     node tools/page/check.mjs [-asks words|key|other|none] <bean-check.html> <bundle folder, .zip or sealed package> [six words]
//
// It takes the loader and the program out of the page, starts the program
// as the page does, and hands it the folder as a browser would. It prints
// the lines of bean check-bundle and exits 0 or 1 the same way. It does
// not run the page's own script: a browser does that.
//
// For one file it does what the page's script does. It asks the check
// first, with no words, what the file asks for. It gives the words only to
// a file that asks for words. With -asks it exits 3 when the check's first
// answer is not the one named: so the page shows the box for words for a
// package that is sealed with words, and for no other file.
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, basename } from "node:path";
import { gunzipSync } from "node:zlib";

const argv = process.argv.slice(2);
let expected = null;
if (argv[0] === "-asks") expected = argv.splice(0, 2)[1];
const [pagePath, target, words] = argv;
if (!pagePath || !target || (expected !== null && !["words", "key", "other", "none"].includes(expected))) {
  console.error("usage: node check.mjs [-asks words|key|other|none] <bean-check.html> <bundle folder or .zip> [six words]");
  process.exit(2);
}
const page = readFileSync(pagePath, "utf8");
const part = (id) => {
  const m = page.match(new RegExp('<script id="' + id + '"[^>]*>([\\s\\S]*?)</script>'));
  if (!m) throw new Error("the page has no " + id);
  return m[1];
};
(0, eval)(part("loader")); // defines Go
const go = new Go();
const { instance } = await WebAssembly.instantiate(gunzipSync(Buffer.from(part("program").trim(), "base64")), go.importObject);
go.run(instance);

let out;
if (statSync(target).isDirectory()) {
  // Every file below the folder, under the folder's own name, as a browser
  // hands over a chosen folder. Links are not followed.
  const paths = [], datas = [];
  const walk = (dir, rel) => {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      if (e.isDirectory()) walk(join(dir, e.name), rel + e.name + "/");
      else if (e.isFile()) { paths.push(rel + e.name); datas.push(new Uint8Array(readFileSync(join(dir, e.name)))); }
    }
  };
  walk(target, basename(target) + "/");
  out = beanCheck(target, null, paths, datas);
} else {
  const data = new Uint8Array(readFileSync(target));
  out = beanCheck(target, data, null, null);
  const first = JSON.parse(out);
  const asks = first.asks || "none";
  if (expected !== null && asks !== expected) {
    console.error("the check says that " + target + " asks for " + asks + ", not for " + expected);
    process.exit(3);
  }
  // A file that asks for words did not open without them.
  if (asks === "words" && !(first.locked && first.unread && !first.ok && !first.rows)) {
    console.error("the check opened " + target + " with no words");
    process.exit(3);
  }
  if (asks === "words" && first.locked && words) out = beanCheck(target, data, null, null, words);
}
const res = JSON.parse(out);
console.log(res.lines.join("\n"));
process.exit(res.ok ? 0 : 1);
