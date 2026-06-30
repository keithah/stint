import { strict as assert } from "node:assert";

const { GET } = require("./route");

void (async () => {
const response = await GET();
const body = await response.text();

assert.equal(response.headers.get("content-type"), "text/x-shellscript; charset=utf-8");
assert.match(body, /repo="keithah\/stint"/);
assert.match(body, /github\.com\/\$repo\/releases\/download\/\$version\/\$asset/);
assert.match(body, /stint_\$\{version\}_\$\{os\}_\$\{arch\}\.tar\.gz/);
assert.match(body, /install_dir="\$\{STINT_INSTALL_DIR:-\$HOME\/\.local\/bin\}"/);
assert.match(body, /\$install_dir\/stint" --version/);
assert.doesNotMatch(body, /go build|make stint/);
})();
