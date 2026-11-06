#!/usr/bin/env node

// Preserve registry originals; this supplement does not rewrite BuildKit attestations.
import { execFileSync } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { baselineMod, verifyUpstream } from './torrent-backport-baseline.mjs';

const IMAGE = 'docker.io/kjlion/kejilion-panel';
const REGISTRY = 'https://registry-1.docker.io';
const TOKEN_URL = 'https://auth.docker.io/token?service=registry.docker.io&scope=repository:kjlion/kejilion-panel:pull';
const DIGEST = /^sha256:[a-f0-9]{64}$/;
const SHA = /^[a-f0-9]{40}$/;
const ARCHES = ['amd64', 'arm64'];
const MODULE = 'github.com/anacrolix/torrent';
const TORRENT_VERSION = 'v1.61.1-0.20260911233437-76452a2c8a2f';
const TORRENT_SUM = 'h1:Qd2S/HI+0pR+EMyPS58AB573G3W92zSngOmH/e0qqP8=';
const MAIN = 'github.com/kejilion/kejilion-panel';
const INDEX_TYPES = ['application/vnd.oci.image.index.v1+json', 'application/vnd.docker.distribution.manifest.list.v2+json'];
const MANIFEST_TYPES = ['application/vnd.oci.image.manifest.v1+json', 'application/vnd.docker.distribution.manifest.v2+json'];
const ACCEPT = [...INDEX_TYPES, ...MANIFEST_TYPES].join(', ');
const MAX_JSON = 32 * 1024 * 1024;
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const requireValue = (condition, message) => { if (!condition) throw new Error(message); };
const decode = bytes => new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes);
const record = bytes => ({ digest: 'sha256:' + hash(bytes), size: bytes.length, rawUTF8: decode(bytes) });

function descriptor(value, limit = MAX_JSON) {
  requireValue(DIGEST.test(value?.digest ?? '') && Number.isSafeInteger(value.size) && value.size > 0 && value.size <= limit,
    'Invalid or oversized registry descriptor');
  return value;
}

export function validateOptions(options) {
  requireValue(SHA.test(options.candidate ?? ''), 'Candidate must be an exact commit');
  requireValue(/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-rc\.[1-9][0-9]*)?$/.test(options.version ?? ''), 'Invalid release version');
  requireValue(options.image === IMAGE && DIGEST.test(options.digest ?? ''), 'Only the official image and an immutable digest are accepted');
}

export function allowedRedirect(url) {
  const value = new URL(url);
  return value.protocol === 'https:' && !value.username && !value.password && (!value.port || value.port === '443')
    && (value.origin === REGISTRY || ['production.cloudflare.docker.com', 'production.cloudfront.docker.com',
      'docker-images-prod.6aa30f8b08e16409b46e0173d6de2f56.r2.cloudflarestorage.com'].includes(value.hostname));
}

// One deadline covers redirects and body consumption. Tokens and signed URLs never enter evidence/errors.
export async function boundedGet(url, { authorization, limit = MAX_JSON, fetcher = fetch, timeoutMs = 45_000 } = {}) {
  const signal = AbortSignal.timeout(timeoutMs);
  let current = url;
  for (let redirects = 0; redirects <= 3; redirects++) {
    const headers = { Accept: ACCEPT, 'User-Agent': 'KPanel-modified-component-provenance', 'Accept-Encoding': 'identity' };
    if (authorization && new URL(current).origin === REGISTRY) headers.Authorization = authorization;
    let response;
    try { response = await fetcher(current, { headers, redirect: 'manual', signal }); }
    catch { throw new Error('Registry request failed or timed out'); }
    if ([301, 302, 303, 307, 308].includes(response.status)) {
      const location = response.headers.get('location');
      await response.body?.cancel();
      requireValue(location && redirects < 3, 'Registry redirect limit exceeded');
      const next = new URL(location, current);
      requireValue(allowedRedirect(next), 'Untrusted registry redirect');
      current = next.href;
      continue;
    }
    requireValue(response.status === 200, 'Registry HTTP status ' + response.status);
    const length = response.headers.get('content-length');
    requireValue(length === null || (/^\d+$/.test(length) && Number(length) <= limit), 'Registry response exceeds size limit');
    requireValue(response.body, 'Missing registry response body');
    const chunks = [];
    let size = 0;
    try {
      for await (const chunk of response.body) {
        size += chunk.length;
        requireValue(size <= limit, 'Registry response exceeds size limit');
        chunks.push(Buffer.from(chunk));
      }
    } catch (error) {
      if (error.message === 'Registry response exceeds size limit') throw error;
      throw new Error('Registry body failed or timed out');
    }
    requireValue(length === null || size === Number(length), 'Registry response size mismatch');
    return { bytes: Buffer.concat(chunks), headers: response.headers };
  }
  throw new Error('Registry redirect limit exceeded');
}

export async function registryClient(fetcher = fetch) {
  const tokenResponse = await boundedGet(TOKEN_URL, { limit: 64 * 1024, fetcher });
  const token = JSON.parse(decode(tokenResponse.bytes)).token;
  requireValue(typeof token === 'string' && token.length > 0 && token.length < 32 * 1024 && !/[\r\n]/.test(token), 'Invalid anonymous registry token');
  let total = 0;
  return async (kind, ref, expectedSize) => {
    requireValue(['manifests', 'blobs'].includes(kind) && DIGEST.test(ref), 'Invalid registry object identity');
    const response = await boundedGet(`${REGISTRY}/v2/kjlion/kejilion-panel/${kind}/${ref}`, { authorization: 'Bearer ' + token, fetcher });
    total += response.bytes.length;
    requireValue(total <= 128 * 1024 * 1024, 'Registry aggregate response budget exceeded');
    const original = record(response.bytes);
    requireValue(original.digest === ref && (expectedSize === undefined || expectedSize === original.size), 'Registry object digest or size mismatch');
    if (kind === 'manifests') requireValue(response.headers.get('docker-content-digest') === ref, 'Registry manifest header digest mismatch');
    return { original, body: JSON.parse(original.rawUTF8) };
  };
}

export function validateStatement(statement, predicateType, childDigest, options) {
  requireValue(['https://in-toto.io/Statement/v0.1', 'https://in-toto.io/Statement/v1'].includes(statement?._type)
    && statement.predicateType === predicateType && statement.subject?.length === 1
    && statement.subject[0]?.digest?.sha256 === childDigest.slice(7), 'Attestation subject or predicate identity mismatch');
  const predicate = statement.predicate;
  if (predicateType === 'https://spdx.dev/Document') {
    requireValue(/^SPDX-2\.[23]$/.test(predicate?.spdxVersion ?? '') && predicate.SPDXID === 'SPDXRef-DOCUMENT'
      && typeof predicate.documentNamespace === 'string' && predicate.documentNamespace.length > 0
      && Array.isArray(predicate.packages) && predicate.packages.length > 0, 'Invalid SPDX SBOM');
    return { kind: 'sbom' };
  }
  let args, materials, buildType, expectedBuildType;
  if (predicateType === 'https://slsa.dev/provenance/v0.2') {
    args = predicate?.invocation?.parameters?.args;
    materials = predicate?.materials;
    buildType = predicate?.buildType;
    expectedBuildType = 'https://mobyproject.org/buildkit@v1';
  } else if (predicateType === 'https://slsa.dev/provenance/v1') {
    // BuildKit v0.30.0 solver/llbsolver/provenance/types/types.go defines this v1 shape.
    args = predicate?.buildDefinition?.externalParameters?.request?.args;
    materials = predicate?.buildDefinition?.resolvedDependencies;
    buildType = predicate?.buildDefinition?.buildType;
    expectedBuildType = 'https://github.com/moby/buildkit/blob/master/docs/attestations/slsa-definitions.md';
  } else throw new Error('Unsupported attestation predicate');
  requireValue(buildType === expectedBuildType && args?.['build-arg:REVISION'] === options.candidate
    && args?.['build-arg:VERSION'] === options.version, 'SLSA build argument identity mismatch');
  requireValue(Array.isArray(materials), 'Missing SLSA material inventory');
  const gitMaterials = materials.filter(item => typeof item?.uri === 'string' && /github\.com[/:]kejilion\/KPanel(?:\.git)?(?:[@#?]|$)/i.test(item.uri));
  for (const material of gitMaterials) requireValue(Object.values(material.digest ?? {}).includes(options.candidate), 'SLSA Git material revision mismatch');
  return { kind: 'slsa', gitMaterial: gitMaterials.length ? 'present-and-matched' : 'absent-in-original',
    identityBinding: 'original-build-args-and-runtime-config-labels' };
}

export function parseGoBuildInfo(raw, arch, executable) {
  const rows = raw.split(/\r?\n/).map(line => line.trim().split(/\t+/));
  const one = name => { const values = rows.filter(row => row[0] === name); requireValue(values.length === 1, 'Missing or duplicate Go ' + name); return values[0]; };
  requireValue(one('path')[1] === MAIN + '/cmd/' + executable && one('mod')[1] === MAIN, 'Go binary main module identity mismatch');
  const builds = rows.filter(row => row[0] === 'build').map(row => row.slice(1).join('\t'));
  for (const setting of ['GOOS=linux', 'GOARCH=' + arch]) {
    const values = builds.filter(value => value.startsWith(setting.split('=')[0] + '='));
    requireValue(values.length === 1 && values[0] === setting, 'Go binary platform mismatch');
  }
  const deps = rows.map((row, index) => ({ row, index })).filter(({ row }) => row[0] === 'dep' && row[1] === MODULE);
  requireValue(deps.length <= 1, 'Duplicate torrent binary dependency');
  if (deps.length) {
    const { row, index } = deps[0];
    requireValue(row[2] === TORRENT_VERSION && row[3] === TORRENT_SUM && rows[index + 1]?.[0] !== '=>', 'Binary upstream torrent identity mismatch');
  }
  requireValue(executable !== 'paneld' || deps.length === 1, 'Panel binary does not link the approved upstream torrent');
  return { mainPackage: MAIN + '/cmd/' + executable, mainModule: MAIN, goos: 'linux', goarch: arch,
    torrent: deps.length ? { module: MODULE, version: TORRENT_VERSION, sum: TORRENT_SUM } : null,
    torrentStatus: deps.length ? 'approved-upstream-linked' : 'not-linked', rawUTF8: raw, sha256: hash(raw) };
}

function command(program, argv, cwd) {
  try { return execFileSync(program, argv, { cwd, encoding: 'utf8', timeout: program === 'docker' ? 300_000 : 30_000,
    maxBuffer: 4 * 1024 * 1024, windowsHide: true }); }
  catch { throw new Error('Provenance command failed: ' + program + ' ' + argv[0]); }
}

export function extractBinaries(options, arch, childDigest, configDigest, run = command) {
  const directory = mkdtempSync(join(tmpdir(), 'kpanel-image-provenance-'));
  const cidfile = join(directory, 'container-id');
  const nonce = randomUUID();
  let container, primaryError;
  const cleanupErrors = [];
  try {
    const output = run('docker', ['create', '--platform', 'linux/' + arch, '--network', 'none', '--cidfile', cidfile,
      '--label', 'io.kejilion.provenance-extraction=' + nonce, options.image + '@' + childDigest], options.repo);
    container = output.trim();
    requireValue(/^[a-f0-9]{64}$/.test(container), 'Invalid created container ID');
    const inspected = JSON.parse(run('docker', ['inspect', '--format', '{{json .}}', container], options.repo));
    requireValue(inspected.Image === configDigest && inspected.Config?.Image === options.image + '@' + childDigest
      && inspected.State?.Running === false && inspected.Config?.Labels?.['io.kejilion.provenance-extraction'] === nonce,
    'Created container image identity mismatch');
    const binaries = [];
    for (const [path, executable] of [['/paneld', 'paneld'], ['/release/kejilion-agent', 'kejilion-agent']]) {
      const target = join(directory, executable);
      run('docker', ['cp', container + ':' + path, target], options.repo);
      const info = lstatSync(target);
      requireValue(info.isFile() && !info.isSymbolicLink() && info.size > 0 && info.size <= 256 * 1024 * 1024, 'Invalid extracted binary');
      const sha256 = hash(readFileSync(target));
      const buildInfo = parseGoBuildInfo(run('go', ['version', '-m', target], options.repo), arch, executable);
      binaries.push({ path, size: info.size, sha256, buildInfo });
    }
    return binaries;
  } catch (error) { primaryError = error; throw error; }
  finally {
    try {
      // A successful Docker create can leave its cidfile even if the client timed out.
      const id = existsSync(cidfile) ? readFileSync(cidfile, 'utf8').trim() : container;
      if (id) {
        requireValue(/^[a-f0-9]{64}$/.test(id), 'Unsafe cleanup container ID');
        const owned = JSON.parse(run('docker', ['inspect', '--format', '{{json .Config.Labels}}', id], options.repo));
        requireValue(owned?.['io.kejilion.provenance-extraction'] === nonce, 'Refusing cleanup of an unowned container');
        run('docker', ['rm', id], options.repo);
      }
    } catch (error) { cleanupErrors.push(error); }
    try {
      requireValue(dirname(resolve(directory)) === resolve(tmpdir()) && basename(directory).startsWith('kpanel-image-provenance-'), 'Unsafe temporary cleanup path');
      rmSync(directory, { recursive: true, force: true });
    } catch (error) { cleanupErrors.push(error); }
    if (cleanupErrors.length) throw new AggregateError([...(primaryError ? [primaryError] : []), ...cleanupErrors], 'Provenance extraction cleanup failed');
  }
}

export async function collectImage(options, get, extract = extractBinaries) {
  const index = await get('manifests', options.digest);
  requireValue(index.body.schemaVersion === 2 && INDEX_TYPES.includes(index.body.mediaType)
    && index.body.manifests?.length === 4, 'Expected exactly two runtime images and their two attestations');
  const runtime = new Map(), attestations = new Map();
  for (const entry of index.body.manifests) {
    descriptor(entry, 4 * 1024 * 1024);
    requireValue(MANIFEST_TYPES.includes(entry.mediaType), 'Unexpected child manifest type');
    const platform = entry.platform;
    if (platform?.os === 'linux' && ARCHES.includes(platform.architecture)) {
      requireValue(!runtime.has(platform.architecture) && (!platform.variant || (platform.architecture === 'arm64' && platform.variant === 'v8')),
        'Duplicate or unexpected runtime platform');
      runtime.set(platform.architecture, entry);
    } else {
      const target = entry.annotations?.['vnd.docker.reference.digest'];
      requireValue(platform?.os === 'unknown' && platform.architecture === 'unknown' && DIGEST.test(target ?? '')
        && entry.annotations?.['vnd.docker.reference.type'] === 'attestation-manifest' && !attestations.has(target), 'Invalid attestation reference');
      attestations.set(target, entry);
    }
  }
  requireValue(runtime.size === 2 && attestations.size === 2 && new Set([...runtime.values()].map(value => value.digest)).size === 2,
    'Incomplete or duplicate image platforms');
  const platforms = [];
  for (const arch of ARCHES) {
    const child = runtime.get(arch);
    const attestation = attestations.get(child.digest);
    requireValue(attestation, 'Missing architecture attestation');
    const manifest = await get('manifests', child.digest, child.size);
    requireValue(manifest.body.schemaVersion === 2 && MANIFEST_TYPES.includes(manifest.body.mediaType)
      && Array.isArray(manifest.body.layers) && manifest.body.layers.length > 0 && manifest.body.layers.length <= 128, 'Invalid runtime manifest');
    for (const layer of manifest.body.layers) descriptor(layer, 2 * 1024 * 1024 * 1024);
    const configDescriptor = descriptor(manifest.body.config, 4 * 1024 * 1024);
    const config = await get('blobs', configDescriptor.digest, configDescriptor.size);
    requireValue(config.body.os === 'linux' && config.body.architecture === arch
      && config.body.config?.Labels?.['org.opencontainers.image.version'] === options.version
      && config.body.config?.Labels?.['org.opencontainers.image.revision'] === options.candidate
      && config.body.config?.Labels?.['org.opencontainers.image.source'] === 'https://github.com/kejilion/KPanel', 'Image config identity mismatch');
    const attManifest = await get('manifests', attestation.digest, attestation.size);
    requireValue(attManifest.body.schemaVersion === 2 && MANIFEST_TYPES.includes(attManifest.body.mediaType)
      && Array.isArray(attManifest.body.layers) && attManifest.body.layers.length === 2, 'Expected exactly SBOM and SLSA attestation layers');
    const attConfigDescriptor = descriptor(attManifest.body.config, 4 * 1024 * 1024);
    const attConfig = await get('blobs', attConfigDescriptor.digest, attConfigDescriptor.size);
    const statements = {};
    for (const layer of attManifest.body.layers) {
      descriptor(layer);
      requireValue(layer.mediaType === 'application/vnd.in-toto+json', 'Unexpected attestation layer');
      const statement = await get('blobs', layer.digest, layer.size);
      const interpretation = validateStatement(statement.body, layer.annotations?.['in-toto.io/predicate-type'], child.digest, options);
      requireValue(!statements[interpretation.kind], 'Duplicate attestation predicate');
      statements[interpretation.kind] = { ...statement.original, ...interpretation };
    }
    requireValue(statements.sbom && statements.slsa, 'Both SPDX SBOM and SLSA are required');
    platforms.push({ os: 'linux', architecture: arch, manifest: manifest.original, config: config.original,
      attestationManifest: attManifest.original, attestationConfig: attConfig.original, statements });
  }
  // Fetch both architectures before potentially slow Docker pulls consume the anonymous token's lifetime.
  for (const platform of platforms) platform.binaries = extract(options, platform.architecture, platform.manifest.digest, platform.config.digest);
  return { repository: options.image, index: index.original, platforms };
}

export async function main(argv) {
  const options = { repo: resolve(import.meta.dirname, '..') };
  for (let i = 0; i < argv.length; i += 2) {
    const name = argv[i]?.slice(2);
    requireValue(['candidate', 'version', 'image', 'digest', 'output'].includes(name) && argv[i] === '--' + name
      && !Object.hasOwn(options, name) && argv[i + 1], 'Usage: torrent-backport-provenance.mjs --candidate SHA --version VERSION --image IMAGE --digest SHA256 --output release/KPANEL_MODIFIED_COMPONENTS.json');
    options[name] = argv[i + 1];
  }
  validateOptions(options);
  requireValue(options.output && resolve(options.output) === join(options.repo, 'release/KPANEL_MODIFIED_COMPONENTS.json'), 'Output must be the canonical release asset');
  requireValue(command('git', ['rev-parse', 'HEAD'], options.repo).trim() === options.candidate
    && readFileSync(join(options.repo, 'VERSION'), 'utf8').trim() === options.version, 'Checkout candidate or version mismatch');
  command('git', ['diff', '--quiet', 'HEAD', '--'], options.repo);
  const { provenance } = verifyUpstream(options.repo);
  baselineMod(readFileSync(join(options.repo, 'go.mod'), 'utf8'));
  const image = await collectImage(options, await registryClient());
  const artifact = { schemaVersion: 1, kind: 'kpanel-modified-components', candidateCommit: options.candidate,
    version: options.version, generatedAt: new Date().toISOString(), component: { ...provenance, modifications: [] }, image,
    interpretation: 'Upstream component identity; no local torrent modifications. Original registry SBOM/SLSA bytes are preserved and remain unmodified; absent Git materials are explicitly recorded.' };
  const directory = join(options.repo, 'release');
  if (!existsSync(directory)) mkdirSync(directory);
  requireValue(lstatSync(directory).isDirectory() && !lstatSync(directory).isSymbolicLink(), 'Unsafe release output directory');
  const bytes = Buffer.from(JSON.stringify(artifact, null, 2) + '\n');
  requireValue(bytes.length <= 192 * 1024 * 1024, 'Provenance asset exceeds output budget');
  writeFileSync(options.output, bytes, { flag: 'wx', mode: 0o644 });
  console.log(JSON.stringify({ asset: 'KPANEL_MODIFIED_COMPONENTS.json', digest: 'sha256:' + hash(bytes), size: bytes.length,
    candidateCommit: options.candidate, version: options.version, imageDigest: options.digest }));
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main(process.argv.slice(2)).catch(error => { console.error('torrent_backport_provenance=fail ' + error.message); process.exitCode = 1; });
}
