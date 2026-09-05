import { readFileSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';

// Consume the authoritative installer heredocs; never maintain a second updater.
const sourcePath = process.argv[2];
if (!sourcePath) throw new Error('usage: node scripts/sync-light-node-runtime.mjs PATH_TO_KEJILION_SH --revision SHA | --check');
const source = readFileSync(sourcePath, 'utf8').replace(/\r\n/g, '\n').replace(/^canshu="CN"$/gm, 'canshu="default"');
const checksum = (content) => createHash('sha256').update(content).digest('hex');
const metadataPath = fileURLToPath(new URL('../cmd/kejilion-node/update_runtime/source.json', import.meta.url));
const checking = process.argv.includes('--check');
const revision = checking ? JSON.parse(readFileSync(metadataPath, 'utf8')).revision : process.argv[process.argv.indexOf('--revision') + 1];
if (!/^[a-f0-9]{40}$/.test(revision)) throw new Error('an exact kejilion/sh revision is required');
const metadata = { repository: 'https://github.com/kejilion/sh', revision, scriptSHA256: checksum(source), templates: {} };
for (const [marker, name] of [
  ['KPANEL_NODE_UPDATE', 'update.sh'],
  ['KPANEL_NODE_UPDATE_SERVICE', 'update.service'],
  ['KPANEL_NODE_UPDATE_TIMER', 'update.timer'],
]) {
  let content = source.split(`<<'${marker}'\n`)[1]?.split(`\n${marker}\n`)[0];
  if (!content) throw new Error(`missing authoritative template: ${marker}`);
  if (name === 'update.sh') {
    const lifecycle = source.split("<<'KPANEL_NODE_LIFECYCLE'\n")[1]?.split('\nKPANEL_NODE_LIFECYCLE\n')[0];
    if (!lifecycle) throw new Error('missing authoritative lifecycle lock template');
    content = `#!/bin/bash\n${lifecycle}\n${content}`;
  }
  const path = fileURLToPath(new URL(`../cmd/kejilion-node/update_runtime/${name}`, import.meta.url));
  metadata.templates[name] = checksum(`${content}\n`);
  if (checking) {
    if (readFileSync(path, 'utf8').replace(/\r\n/g, '\n') !== `${content}\n`) throw new Error(`runtime differs from kejilion.sh: ${name}`);
  } else writeFileSync(path, `${content}\n`);
}
if (checking) {
  if (JSON.stringify(JSON.parse(readFileSync(metadataPath, 'utf8'))) !== JSON.stringify(metadata)) throw new Error('runtime source metadata differs from kejilion.sh');
} else writeFileSync(metadataPath, `${JSON.stringify(metadata, null, 2)}\n`);
console.log('Light-node update templates match kejilion.sh.');
