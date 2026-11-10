// check.mjs: runs the check that a built bean-check.html carries, without
// a browser, so that scripts can compare it with bean check-bundle.
//
//     node tools/page/check.mjs <bean-check.html> <bundle folder or .zip>
//
// It takes the loader and the program out of the page, starts the program
// as the page does, and hands it the folder as a browser would. It prints
// the lines of bean check-bundle and exits 0 or 1 the same way. It does
// not run the page's own script: a browser does that.
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, basename } from "node:path";
import { gunzipSync } from "node:zlib";

const [pagePath, target] = process.argv.slice(2);
if (!pagePath || !target) {
  console.error("usage: node check.mjs <bean-check.html> <bundle folder or .zip>");
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
  out = beanCheck(target, new Uint8Array(readFileSync(target)), null, null);
}
const res = JSON.parse(out);
console.log(res.lines.join("\n"));
process.exit(res.ok ? 0 : 1);
