import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const source = readFileSync("components/settings/github-account-card.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /LogOut/);
assert.match(source, /logout/);
assert.match(source, /const signOut = useMutation/);
assert.match(source, /mutationFn: logout/);
assert.match(source, /window\.location\.href = "\/login"/);
assert.match(source, /<LogOut size=\{16\} \/> Sign out/);
assert.match(packageJSON, /lib\/auth-api\.test\.ts/);
assert.match(packageJSON, /app\/\(console\)\/settings\/logout-button\.test\.ts/);
