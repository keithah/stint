import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = readFileSync("app/(console)/leaderboards/page.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /const createRange = normalizeLeaderboardRangeInput\(customRange, range\);/);
assert.match(source, /const canCreateBoard = name\.trim\(\)\.length > 0 && Boolean\(createRange\);/);
assert.match(source, /const activeBoardName = activeBoard\?\.name\.trim\(\) \?\? "";/);
assert.match(source, /const editedBoardName = editName\.trim\(\) \|\| activeBoardName;/);
assert.match(source, /const saveRange = normalizeLeaderboardRangeInput\(editCustomRange, selectedEditRange as StatsRange\);/);
assert.match(source, /const canSaveBoard = Boolean\(activeBoard && editedBoardName && saveRange\);/);
assert.match(source, /createLeaderboard\(name\.trim\(\), createRange as StatsRange\)/);
assert.match(source, /updateLeaderboard\(activeBoard\?\.id \?\? "", editedBoardName, saveRange as StatsRange\)/);
assert.match(source, /disabled=\{create\.isPending \|\| !canCreateBoard\}/);
assert.match(source, /disabled=\{update\.isPending \|\| !canSaveBoard\}/);
assert.match(packageJSON, /app\/\(console\)\/leaderboards\/name-validation\.test\.ts/);
