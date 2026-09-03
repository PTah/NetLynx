/**
 * Одноразовый msg-filter для git filter-branch:
 * удаляет любой Made-with и ставит актуальный трейлер.
 *
 * Пример:
 *   git filter-branch -f --msg-filter "node /absolute/path/to/scripts/git-msg-replace-made-with-trailer.js" -- --all
 */
const fs = require("fs");
const d = fs.readFileSync(0, "utf8");
const cleaned = d
  .split(/\r?\n/)
  .filter((line) => !/^Made-with:/i.test(line.trim()))
  .join("\n")
  .replace(/\s+$/, "");
process.stdout.write(`${cleaned}\n\nMade-with: "Brain, Google and Great Cursor AI"\n`);
