import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const source = readFileSync("app/(console)/reports/page.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /deleteExternalDurationsBulk/);
assert.match(source, /const \[selectedExternalDurationIDs, setSelectedExternalDurationIDs\]/);
assert.match(source, /const deleteSelectedExternalDurations = useMutation/);
assert.match(source, /mutationFn: \(\) => deleteExternalDurationsBulk\(selectedExternalDurationIDs\)/);
assert.match(source, /setSelectedExternalDurationIDs\(\[\]\)/);
assert.match(source, /client\.invalidateQueries\(\{ queryKey: \["external-durations"\] \}\)/);
assert.match(source, /selectedExternalDurationIDSet = useMemo\(\(\) => new Set\(selectedExternalDurationIDs\), \[selectedExternalDurationIDs\]\)/);
assert.match(source, /selectedExternalDurationIDSet\.has\(duration\.id\)/);
assert.match(source, /deleteSelectedExternalDurations\.mutate\(\)/);
assert.match(source, /Delete selected durations/);
assert.match(packageJSON, /scripts\/run-tests\.mjs/);
