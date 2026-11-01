import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

const version = process.argv[2];
if (!/^\d+\.\d+\.\d+$/.test(version || '')) {
  console.error('Usage: node scripts/set-version.mjs 0.1.1');
  process.exit(1);
}
const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const path = join(root, 'wails.json');
const config = JSON.parse(readFileSync(path, 'utf8'));
config.info.productVersion = version;
writeFileSync(path, JSON.stringify(config, null, 2) + '\n');
writeFileSync(join(root, 'VERSION'), version + '\n');
console.log(`Version set to ${version}. Update CHANGELOG.md before releasing.`);
