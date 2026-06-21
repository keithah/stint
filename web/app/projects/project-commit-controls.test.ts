import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const source = readFileSync(join(process.cwd(), "app/projects/[name]/page.tsx"), "utf8");
const packageJSON = readFileSync(join(process.cwd(), "package.json"), "utf8");

assert.match(source, /const \[commitBranch, setCommitBranch\]/);
assert.match(source, /const \[commitPage, setCommitPage\]/);
assert.match(source, /queryFn: \(\) => listProjectCommits\(name, \{ branch: commitBranch \|\| undefined, page: commitPage \}\)/);
assert.match(source, /value=\{commitBranch\}/);
assert.match(source, /onChange=\{\(event\) => \{\s*setCommitBranch\(event\.target\.value\);\s*setCommitPage\(1\);/s);
assert.match(source, /disabled=\{!commits\.data\?\.prev_page\}/);
assert.match(source, /disabled=\{!commits\.data\?\.next_page\}/);
assert.match(source, /setCommitPage\(commits\.data\?\.prev_page \?\? 1\)/);
assert.match(source, /setCommitPage\(commits\.data\?\.next_page \?\? commitPage\)/);
assert.match(source, /href=\{commit\.html_url \|\| commit\.url \|\| undefined\}/);
assert.match(source, /target=\{commit\.html_url \|\| commit\.url \? "_blank" : undefined\}/);
assert.match(packageJSON, /app\/projects\/project-commit-controls\.test\.ts/);
