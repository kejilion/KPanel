import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { basename, dirname, resolve } from 'node:path';
import test from 'node:test';
import { allowedRedirect, boundedGet, collectImage, extractBinaries, main, parseGoBuildInfo,
  registryClient, validateOptions, validateStatement } from '../torrent-backport-provenance.mjs';

const repo = resolve(import.meta.dirname, '../..');
const digest = value => 'sha256:' + createHash('sha256').update(value).digest('hex');
const options = { repo, candidate: 'a'.repeat(40), version: '1.26.0-rc.7', image: 'docker.io/kjlion/kejilion-panel', digest: 'sha256:' + 'b'.repeat(64) };
const manifestType = 'application/vnd.oci.image.manifest.v1+json';
const sbomType = 'https://spdx.dev/Document';
const slsaType = 'https://slsa.dev/provenance/v0.2';
const mainModule = 'github.com/kejilion/kejilion-panel';
const torrentVersion = 'v1.61.1-0.20260911233437-76452a2c8a2f';
const torrentSum = 'h1:Qd2S/HI+0pR+EMyPS58AB573G3W92zSngOmH/e0qqP8=';

function statement(type, subject, predicate) {
  return { _type: 'https://in-toto.io/Statement/v0.1', predicateType: type,
    subject: [{ name: 'fixture-image', digest: { sha256: subject.slice(7) } }], predicate };
}

function fixture(change = () => {}) {
  const objects = new Map();
  const add = (body, kind = 'blobs', mediaType = body.mediaType) => {
    // Deliberate indentation/newline: consumers must retain original bytes.
    const bytes = Buffer.from(JSON.stringify(body, null, 3) + '\n');
    const desc = { digest: digest(bytes), size: bytes.length, mediaType };
    objects.set(`${kind}/${desc.digest}`, bytes);
    return desc;
  };
  const children = [], attested = [];
  for (const arch of ['amd64', 'arm64']) {
    const config = { architecture: arch, os: 'linux', config: { Labels: {
      'org.opencontainers.image.revision': options.candidate, 'org.opencontainers.image.version': options.version,
      'org.opencontainers.image.source': 'https://github.com/kejilion/KPanel' } } };
    change('config', config, arch);
    const childBody = { schemaVersion: 2, mediaType: manifestType, config: add(config),
      layers: [{ digest: digest('runtime-layer-' + arch), size: 123, mediaType: 'application/vnd.oci.image.layer.v1.tar+gzip' }] };
    change('manifest', childBody, arch);
    const child = { ...add(childBody, 'manifests'), platform: { os: 'linux', architecture: arch } };
    children.push(child);
    const sbom = statement(sbomType, child.digest, { spdxVersion: 'SPDX-2.3', SPDXID: 'SPDXRef-DOCUMENT',
      documentNamespace: 'https://fixture.invalid/' + arch, packages: [{ name: 'fixture-package' }] });
    const slsa = statement(slsaType, child.digest, { buildType: 'https://mobyproject.org/buildkit@v1', materials: [],
      invocation: { parameters: { args: { 'build-arg:REVISION': options.candidate, 'build-arg:VERSION': options.version } } } });
    change('sbom', sbom, arch); change('slsa', slsa, arch);
    const layers = [sbom, slsa].map(body => ({ ...add(body, 'blobs', 'application/vnd.in-toto+json'),
      annotations: { 'in-toto.io/predicate-type': body.predicateType } }));
    const attBody = { schemaVersion: 2, mediaType: manifestType, config: add({ architecture: 'unknown', os: 'unknown' }), layers };
    change('attManifest', attBody, arch);
    const att = { ...add(attBody, 'manifests'), platform: { os: 'unknown', architecture: 'unknown' },
      annotations: { 'vnd.docker.reference.type': 'attestation-manifest', 'vnd.docker.reference.digest': child.digest } };
    change('attDescriptor', att, arch);
    attested.push(att);
  }
  const index = { schemaVersion: 2, mediaType: 'application/vnd.oci.image.index.v1+json', manifests: [...children, ...attested] };
  change('index', index);
  const indexDescriptor = add(index, 'manifests');
  const requests = [];
  const fetcher = async (url, init) => {
    requests.push({ url, init });
    if (url.startsWith('https://auth.docker.io/token?')) return new Response(JSON.stringify({ token: 'anonymous-fixture-token' }));
    assert.equal(init.headers.Authorization, 'Bearer anonymous-fixture-token');
    const key = url.split('/v2/kjlion/kejilion-panel/')[1];
    const bytes = objects.get(key);
    assert.ok(bytes, 'unexpected object ' + key);
    return new Response(bytes, { headers: { 'content-length': String(bytes.length), 'docker-content-digest': digest(bytes) } });
  };
  return { options: { ...options, digest: indexDescriptor.digest }, objects, fetcher, requests };
}

function goInfo(arch, executable, withTorrent = executable === 'paneld') {
  return `/temporary/${executable}: go1.27.4\n\tpath\t${mainModule}/cmd/${executable}\n\tmod\t${mainModule}\t(devel)\t\n`
    + (withTorrent ? `\tdep\tgithub.com/anacrolix/torrent\t${torrentVersion}\t${torrentSum}\n` : '')
    + `\tbuild\tGOOS=linux\n\tbuild\tGOARCH=${arch}\n`;
}

test('options restrict repository, immutable digest and exact candidate', () => {
  validateOptions(options);
  for (const patch of [{ image: 'docker.io/other/panel' }, { digest: 'latest' }, { candidate: 'main' }, { version: '1.26.0;echo bad' }]) {
    assert.throws(() => validateOptions({ ...options, ...patch }));
  }
});

test('two architecture evidence retains original registry bytes and subject bindings', async () => {
  const data = fixture();
  const calls = [];
  const image = await collectImage(data.options, await registryClient(data.fetcher), (...args) => { calls.push(args); return [{ fixture: true }]; });
  assert.equal(image.index.rawUTF8, data.objects.get('manifests/' + data.options.digest).toString());
  assert.deepEqual(image.platforms.map(platform => platform.architecture), ['amd64', 'arm64']);
  assert.equal(calls.length, 2);
  for (const platform of image.platforms) {
    assert.equal(calls.find(call => call[1] === platform.architecture)[2], platform.manifest.digest);
    for (const original of [platform.manifest, platform.config, platform.attestationManifest, platform.attestationConfig,
      platform.statements.sbom, platform.statements.slsa]) {
      assert.equal(digest(original.rawUTF8), original.digest);
      assert.equal(Buffer.byteLength(original.rawUTF8), original.size);
    }
    assert.equal(platform.statements.slsa.gitMaterial, 'absent-in-original');
  }
});

for (const [name, section, mutate, error] of [
  ['extra platform', 'index', value => value.manifests.push(value.manifests[0]), /exactly two/],
  ['duplicate platform', 'index', value => { value.manifests[1].platform.architecture = 'amd64'; }, /Duplicate/],
  ['wrong config revision', 'config', value => { value.config.Labels['org.opencontainers.image.revision'] = 'c'.repeat(40); }, /config identity/],
  ['wrong child annotation', 'attDescriptor', value => { value.annotations['vnd.docker.reference.digest'] = 'sha256:' + 'c'.repeat(64); }, /Missing architecture|attestation reference/],
  ['wrong attestation subject', 'sbom', value => { value.subject[0].digest.sha256 = 'c'.repeat(64); }, /subject/],
  ['empty SBOM', 'sbom', value => { value.predicate.packages = []; }, /SPDX/],
  ['wrong SLSA revision', 'slsa', value => { value.predicate.invocation.parameters.args['build-arg:REVISION'] = 'c'.repeat(40); }, /build argument/],
  ['missing SLSA', 'attManifest', value => { value.layers.pop(); }, /exactly SBOM and SLSA/],
  ['duplicate SBOM', 'attManifest', value => { value.layers[1] = value.layers[0]; }, /Duplicate attestation/],
  ['wrong descriptor size', 'index', value => { value.manifests[0].size++; }, /digest or size/],
  ['unknown predicate', 'slsa', value => { value.predicateType = 'https://unknown.invalid/predicate'; }, /Unsupported attestation/],
]) {
  test('image collection rejects ' + name, async () => {
    const data = fixture((part, value, arch) => { if (part === section && (!arch || arch === 'amd64')) mutate(value); });
    await assert.rejects(collectImage(data.options, await registryClient(data.fetcher), () => []), error);
  });
}

test('SLSA v1 uses the exact BuildKit request shape, and records available Git material', () => {
  const value = statement('https://slsa.dev/provenance/v1', options.digest, { buildDefinition: {
    buildType: 'https://github.com/moby/buildkit/blob/master/docs/attestations/slsa-definitions.md',
    externalParameters: { request: { args: { 'build-arg:REVISION': options.candidate, 'build-arg:VERSION': options.version } } },
    resolvedDependencies: [{ uri: 'git+https://github.com/kejilion/KPanel.git', digest: { sha1: options.candidate } }] } });
  assert.equal(validateStatement(value, value.predicateType, options.digest, options).gitMaterial, 'present-and-matched');
  value.predicate.buildDefinition.resolvedDependencies[0].digest.sha1 = 'c'.repeat(40);
  assert.throws(() => validateStatement(value, value.predicateType, options.digest, options), /Git material/);
  value.predicate.buildDefinition.externalParameters.args = value.predicate.buildDefinition.externalParameters.request.args;
  delete value.predicate.buildDefinition.externalParameters.request;
  assert.throws(() => validateStatement(value, value.predicateType, options.digest, options), /build argument/);
});

test('registry rejects corrupt bytes, manifest header mismatch, oversized and invalid UTF-8 bodies', async () => {
  const expected = digest('{}');
  for (const [bytes, headers, error] of [
    ['{ }', { 'docker-content-digest': expected }, /digest or size/],
    ['{}', { 'docker-content-digest': digest('bad') }, /header digest/],
    [Buffer.from([0xff]), {}, /encoded data/],
  ]) {
    const client = await registryClient(async url => url.startsWith('https://auth.') ? new Response('{"token":"fixture"}') : new Response(bytes, { headers }));
    await assert.rejects(client('manifests', expected), error);
  }
  await assert.rejects(boundedGet('https://registry-1.docker.io/test', { limit: 3, fetcher: async () => new Response('1234') }), /size limit/);
  await assert.rejects(boundedGet('https://registry-1.docker.io/test', { limit: 3, fetcher: async () => new Response('1', { headers: { 'content-length': '4' } }) }), /size limit/);
  await assert.rejects(boundedGet('https://registry-1.docker.io/test', { fetcher: async (_url, init) => { assert.ok(init.signal instanceof AbortSignal); throw new Error('secret URL'); } }), /^Error: Registry request failed or timed out$/);
});

test('trusted CDN redirect strips registry bearer; untrusted targets and loops fail', async () => {
  for (const host of ['production.cloudflare.docker.com', 'production.cloudfront.docker.com',
    'docker-images-prod.6aa30f8b08e16409b46e0173d6de2f56.r2.cloudflarestorage.com']) {
    const requests = [];
    const fetcher = async (url, init) => {
      requests.push({ url, headers: init.headers });
      return requests.length === 1 ? new Response(null, { status: 307, headers: { location: `https://${host}/fixture?signed=redacted` } }) : new Response('{}');
    };
    await boundedGet('https://registry-1.docker.io/test', { authorization: 'Bearer fixture-secret', fetcher });
    assert.equal(requests[0].headers.Authorization, 'Bearer fixture-secret');
    assert.equal(requests[1].headers.Authorization, undefined);
  }
  for (const url of ['http://production.cloudflare.docker.com/a', 'https://127.0.0.1/a', 'https://evil.invalid/a',
    'https://x@production.cloudflare.docker.com/a', 'https://untrusted.r2.cloudflarestorage.com/a',
    'https://docker-images-prod-aa.bb.r2.cloudflarestorage.com/a']) assert.equal(allowedRedirect(url), false);
  await assert.rejects(boundedGet('https://registry-1.docker.io/test', { fetcher: async () => new Response(null, { status: 307, headers: { location: 'https://evil.invalid/a' } }) }), /Untrusted/);
  await assert.rejects(boundedGet('https://registry-1.docker.io/test', { fetcher: async () => new Response(null, { status: 307, headers: { location: 'https://registry-1.docker.io/again' } }) }), /redirect limit/);
});

test('binary build info enforces main module/platform and upstream identity; Agent absence is explicit', () => {
  assert.equal(parseGoBuildInfo(goInfo('amd64', 'paneld'), 'amd64', 'paneld').torrent.version, torrentVersion);
  assert.equal(parseGoBuildInfo(goInfo('arm64', 'kejilion-agent'), 'arm64', 'kejilion-agent').torrentStatus, 'not-linked');
  for (const bad of [goInfo('arm64', 'paneld'), goInfo('amd64', 'paneld', false),
    goInfo('amd64', 'paneld').replace(torrentVersion, 'v1.60.0'), goInfo('amd64', 'paneld').replace(torrentSum, 'h1:invalid'),
    goInfo('amd64', 'paneld').replace(`\t${torrentSum}\n`, `\t${torrentSum}\n\t=>\t./third_party/torrent\t(devel)\n`),
    goInfo('amd64', 'paneld').replace(mainModule + '/cmd/paneld', 'other/cmd/paneld'), goInfo('amd64', 'paneld') + '\tbuild\tGOARCH=arm64\n']) {
    assert.throws(() => parseGoBuildInfo(bad, 'amd64', 'paneld'));
  }
});

function binaryRunner(failure) {
  const calls = [];
  let directory, nonce;
  const id = 'd'.repeat(64);
  const run = (program, argv) => {
    calls.push({ program, argv });
    if (program === 'docker' && argv[0] === 'create') {
      const cidfile = argv[argv.indexOf('--cidfile') + 1]; directory = dirname(cidfile);
      nonce = argv[argv.indexOf('--label') + 1].split('=')[1];
      writeFileSync(cidfile, id);
      if (failure === 'create') throw new Error('create interrupted after server success');
      return id + '\n';
    }
    if (program === 'docker' && argv[0] === 'inspect') {
      const labels = { 'io.kejilion.provenance-extraction': failure === 'unowned' ? 'different' : nonce };
      if (argv[2] === '{{json .Config.Labels}}') return JSON.stringify(labels);
      return JSON.stringify({ Image: options.digest, Config: { Image: options.image + '@' + options.digest, Labels: labels }, State: { Running: false } });
    }
    if (program === 'docker' && argv[0] === 'cp') {
      if (failure === 'cp') throw new Error('copy failed');
      writeFileSync(argv[2], 'fixture-binary-' + basename(argv[2])); return '';
    }
    if (program === 'go') {
      if (failure === 'go') throw new Error('build info failed');
      return goInfo('arm64', basename(argv[2]));
    }
    if (program === 'docker' && argv[0] === 'rm') { if (failure === 'rm') throw new Error('cleanup failed'); return ''; }
    throw new Error('Unexpected command');
  };
  return { run, calls, directory: () => directory, id };
}

test('binary extraction never runs the image and cleans only its created container and temporary files', () => {
  const mock = binaryRunner();
  const binaries = extractBinaries(options, 'arm64', options.digest, options.digest, mock.run);
  assert.equal(binaries.length, 2);
  assert.equal(binaries[1].buildInfo.torrentStatus, 'not-linked');
  assert.equal(existsSync(mock.directory()), false);
  assert.deepEqual(mock.calls.filter(call => call.program === 'docker' && call.argv[0] === 'rm').map(call => call.argv), [['rm', mock.id]]);
  assert.ok(mock.calls.every(call => !['run', 'start', 'exec', 'prune'].includes(call.argv[0])));
  assert.ok(mock.calls.find(call => call.argv[0] === 'create').argv.includes(options.image + '@' + options.digest));
});

for (const failure of ['create', 'cp', 'go', 'rm', 'unowned']) {
  test('extraction cleanup after ' + failure + ' failure is bounded and fail-closed', () => {
    const mock = binaryRunner(failure);
    assert.throws(() => extractBinaries(options, 'arm64', options.digest, options.digest, mock.run));
    assert.equal(existsSync(mock.directory()), false);
    assert.equal(mock.calls.filter(call => call.argv[0] === 'rm').length, failure === 'unowned' ? 0 : 1);
  });
}

test('CLI rejects a mismatched checkout before registry or Docker work', async () => {
  await assert.rejects(main(['--candidate', options.candidate, '--version', options.version, '--image', options.image,
    '--digest', options.digest, '--output', resolve(repo, 'release/KPANEL_MODIFIED_COMPONENTS.json')]), /Checkout candidate or version mismatch/);
});

test('workflow generation, draft upload and digest/size verification precede both publication steps', () => {
  const governance = readFileSync(resolve(repo, 'scripts/verify-governance.sh'), 'utf8');
  assert.match(governance, /^  scripts\/tests\/torrent-backport-provenance\.test\.mjs \\$/m);
  const workflow = readFileSync(resolve(repo, '.github/workflows/release.yml'), 'utf8');
  const push = workflow.indexOf('- name: Build and push multi-architecture image');
  const gate = workflow.indexOf('- name: Verify and upload modified-component provenance');
  const promote = workflow.indexOf('- name: Promote image to its release channel');
  const publish = workflow.indexOf('- name: Publish GitHub release');
  assert.ok(push < gate && gate < promote && promote < publish);
  const step = workflow.slice(gate, workflow.indexOf('- name: Validate final release notes', gate));
  assert.match(step, /set -euo pipefail/);
  assert.match(step, /--candidate "\$RELEASE_SHA" --version "\$VERSION"/);
  assert.match(step, /gh release upload/);
  assert.match(step, /\.draft == true and \.tag_name == \$tag/);
  assert.match(step, /\.\[0\]\.digest == \$digest and \.\[0\]\.size == \$size/);
  assert.doesNotMatch(step, /continue-on-error|if:\s*always|SHA256SUMS/);
  for (const content of [workflow.slice(promote, publish), workflow.slice(publish)]) assert.match(content, /test "\$BACKPORT_PROVENANCE_VERIFIED" = true/);
});
