import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

// Freeze source identity only; this does not run the target or prove audit coverage.
export function auditBaseline(repo, ref) {
  if (!ref || ref.startsWith('-')) throw new Error('baseline ref is required and must not be an option');
  const git = (...args) => execFileSync('git', ['-C', repo, ...args], { encoding: 'utf8' }).trim();
  const sourceRef = git('rev-parse', '--verify', '--end-of-options', `${ref}^{commit}`);
  const head = git('rev-parse', 'HEAD');
  if (head !== sourceRef) throw new Error('baseline does not match worktree HEAD');
  if (git('status', '--porcelain=v1', '--untracked-files=all', '--ignore-submodules=none')) {
    throw new Error('audit source must be clean; preserve changes in a dedicated commit or use a clean worktree');
  }
  return { source_ref: sourceRef, source_tree: git('rev-parse', 'HEAD^{tree}'), source_dirty: false };
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    const [repo, ref, ...extra] = process.argv.slice(2);
    if (!repo || !ref || extra.length) throw new Error('usage: node scripts/security-audit-baseline.mjs <source-worktree> <baseline-ref>');
    process.stdout.write(`${JSON.stringify(auditBaseline(repo, ref), null, 2)}\n`);
  } catch (error) {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  }
}
