/**
 * git filter-branch --msg-filter: оставляет ровно один trailer
 * «Made-with: "Brain, Google and Great Cursor AI"».
 */
const fs = require("fs");
let d = fs.readFileSync(0, "utf8");
const lines = d.split(/\n/).filter((line) => !/^Made-with:/i.test(line.trim()));
d = lines.join("\n").replace(/\s+$/, "") + '\n\nMade-with: "Brain, Google and Great Cursor AI"\n';
process.stdout.write(d);
