import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = readFileSync("app/settings/page.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /const canSaveProfile = .*profile\.timeout_minutes <= 120.*profile\.heartbeat_retention_days >= 0.*profile\.public_username\?\.trim\(\)/s);
assert.match(source, /\^\[A-Za-z0-9\]\[A-Za-z0-9_-\]\{1,37\}\[A-Za-z0-9\]\$/);
assert.match(source, /mutationFn: \(\) =>\s+updateUser\(\{\s+\.\.\.profile,\s+timezone: profile\.timezone\.trim\(\),\s+country: profile\.country\?\.trim\(\)\.toUpperCase\(\),\s+timeout_minutes: Math\.min\(120, Math\.max\(0, profile\.timeout_minutes\)\),\s+heartbeat_retention_days: Math\.max\(0, profile\.heartbeat_retention_days\),\s+public_username: profile\.public_username\?\.trim\(\)\.replace/s);
assert.match(source, /public_project_visibility: profile\.public_project_visibility/);
assert.match(source, /public_show_ai: profile\.public_show_ai/);
assert.match(source, /disabled=\{saveProfile\.isPending \|\| !canSaveProfile\}/);
assert.match(source, /min=\{0\}/);
assert.match(source, /onChange=\{\(event\) => setProfileDraft\(\{ \.\.\.profile, timeout_minutes: Math\.min\(120, Math\.max\(0, Number\(event\.target\.value\) \|\| 0\)\) \}\)\}/);
assert.match(source, /onChange=\{\(event\) => setProfileDraft\(\{ \.\.\.profile, heartbeat_retention_days: Math\.max\(0, Number\(event\.target\.value\) \|\| 0\) \}\)\}/);
assert.match(packageJSON, /app\/settings\/profile-validation\.test\.ts/);
