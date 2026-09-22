#!/usr/bin/env python3
from pathlib import Path
import argparse
import json,hashlib,re,subprocess,shutil
parser=argparse.ArgumentParser(description='Regenerate SCA provenance from pinned module-external references and preserved baseline evidence.');parser.add_argument('--references',type=Path,required=True);parser.add_argument('--evidence',type=Path,required=True);args=parser.parse_args()
root=Path.cwd(); sca=root/'common/sca'; ref=args.references.resolve(); evidence=args.evidence.resolve(); baseline='d33a21b6a30945df93a663a18f3cce087b0f3718'
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def write(name,data):p=sca/name;p.parent.mkdir(parents=True,exist_ok=True);p.write_text(json.dumps(data,indent=2,ensure_ascii=False)+'\n')
formats=[('DPKG','dpkg','dpkg-pkg','Debian status paragraphs; Depends AND/OR','analyzer/dpkg.go'),('RPM','rpm','rpm-pkg','BDB hash, NDB, SQLite Packages snapshots; 16 real layouts','core/rpm'),('APK','apk','apk-pkg','Alpine installed text records','analyzer/apk.go'),('Ruby Bundler','ruby_bundler','ruby-bundler-lang','GEM/GIT/PATH lock sections; platform variants','analyzer/dep-parser/ruby/bundler'),('Rust Cargo','rust_cargo','rust-cargo-lang','Cargo.lock v1/v2/v3/v4 package arrays; exact encoded source-qualified refs','analyzer/dep-parser/rust/cargo'),('Ruby GemSpec','ruby_gemspec','ruby-gemspec-lang','Static quoted assignments with optional .freeze; no Ruby evaluation','analyzer/dep-parser/ruby/gemspec'),('Python Poetry','python_poetry','python-poetry-lang','Poetry legacy layouts and lock-version 2.1; groups, group markers and dependency alternatives','analyzer/dep-parser/python/poetry'),('Python Pipenv','python_pipenv','python-pipenv-lang','Pipfile.lock JSON default group; index and markers','analyzer/dep-parser/python/pipenv'),('Python Pip','python_pip','python-pip-lang','Named requirements, == pins, ranges, extras, markers, URLs, hashes; includes/options rejected','core/pyrequire'),('Python Packaging','python_packaging','python-packaging-lang','egg/egg-info/dist-info PKG-INFO/METADATA, License-Expression, Requires-Dist','analyzer/dep-parser/python/packaging'),('PHP Composer','php_composer','php-composer-lang','composer.json declarations; composer.lock runtime packages and require','analyzer/dep-parser/php/composer'),('Node Yarn','node_yarn','yarm-lang','Yarn v1/v2 npm descriptors; resolved, dependencies and optionalDependencies','analyzer/dep-parser/nodejs/yarn'),('Node pnpm','node_pnpm','npmp-lang','pnpm lock v5/v6 and v9.0; v9 metadata/snapshot join, peer contexts, aliases and importer declarations','analyzer/dep-parser/nodejs/pnpm'),('Node npm','node_npm','npm-lang','package.json declarations; package-lock v1/v2/v3; workspace links','analyzer/dep-parser/nodejs/npm'),('Java POM','java_pom','pom-lang','POM XML; explicit relative parents/modules/BOM and repository material only','analyzer/dep-parser/java/pom'),('Java Gradle','java_gradle','gradle-lang','Gradle dependency lock group:name:version=scopes lines','analyzer/dep-parser/java/gradle'),('Java JAR','java_jar','jar-lang','JAR/WAR/EAR metadata and bounded nested ZIPs','analyzer/parser_java_jar.go'),('Go mod','go_mod','go-mod-lang','module/go/toolchain/godebug/require/replace/exclude/retract and exact go.sum checksum evidence','core/gomod'),('Go binary','go_binary','go-binary-lang','Go buildinfo in ELF/PE/Mach-O as supported by Go 1.22 debug/buildinfo','analyzer/dep-parser/golang/binary'),('C/C++ Conan','conan','conan-lang','Conan graph_lock.nodes JSON; native graph IDs, full ref provenance','analyzer/dep-parser/c/conan')]
# Use actual exported identifiers, avoiding task prose spelling ambiguities.
for i,(title,fixture,typ,boundary,impl) in enumerate(formats):
 src=list((sca/'analyzer').glob('*.go'));needle={'Node Yarn':'nodejs_yarn.go','Node pnpm':'nodejs_pnpm.go','Node npm':'nodejs_npm.go','Java POM':'java_pom.go','Java Gradle':'java_gradle.go','Java JAR':'java_jar.go','Go mod':'go_mod.go','Go binary':'go_binary.go','C/C++ Conan':'c_conan.go','Python Poetry':'python_poerty.go'}.get(title,fixture+'.go')
 p=sca/'analyzer'/needle
 if p.exists():
  m=re.search(r'Typ\w+\s+TypAnalyzer\s*=\s*"([^"]+)"',p.read_text());typ=m.group(1) if m else typ
 formats[i]=(title,fixture,typ,boundary,impl)
contracts=[]
for i,(title,fixture,typ,boundary,impl) in enumerate(formats,1):
 tests=[str(p.relative_to(sca)) for p in (sca/impl).rglob('*test.go')] if (sca/impl).is_dir() else []
 tests += (['analyzer/upstream_jar_test.go'] if title=='Java JAR' else [])
 tests += ['scan_contract_test.go','report_contract_test.go','format_fuzz_test.go']
 contracts.append(dict(id=f'SCA-{i:02}',name=title,analyzer=typ,status='RETAIN',grammar=boundary,implementation=impl,fixtures_root='testdata/'+fixture,tests=tests,identity='ecosystem/name/version/source/architecture/variant/verification',occurrence='snapshot/project/file/native ID/source ranges/evidence',unknown_version='empty version, raw constraint in Requirement',failure='structured Diagnostic plus Complete=false and non-nil error; partial evidence allowed',io='caller-supplied immutable fs.FS; no acquisition',budget_policy='resource-policy.json'))
modern_archives={'Rust Cargo':'cargo_lock_v4_corpus_v1.zip','Node pnpm':'pnpm_lock_v9_corpus_v1.zip','Python Poetry':'poetry_lock_v2_1_corpus_v1.zip'}
for contract in contracts:
 if contract['name'] in modern_archives:
  contract['tests'].append('modern_lock_test.go')
  contract['additional_fixture_archives']=['testdata/'+modern_archives[contract['name']]]
write('function-contracts.json',dict(schema_version=1,baseline=baseline,contracts=contracts,additional_contracts=json.loads((sca/'function-contracts.json').read_text()).get('additional_contracts',[]),removed=['Docker context image/container/file acquisition','Docker endpoint option','Git repository/history acquisition'],not_promised=['full ecosystem solving','arbitrary Ruby/Groovy/Python execution','arbitrary YAML/TOML 1.1','SQLite WAL recovery','host mutable filesystem race-proof sandbox']))
# Versioned policy mirrors Normalize. Explicit scopes prevent unsupported claims.
defaults={'MaxFiles':200000,'MaxCandidateFiles':10000,'MaxFileBytes':64<<20,'MaxTotalReadBytes':512<<20,'MaxComponents':200000,'MaxObservations':400000,'MaxEdges':1000000,'MaxArchiveEntries':100000,'MaxArchiveDepth':8,'MaxExpandedBytes':512<<20,'MaxFieldBytes':1<<20,'MaxSyntaxDepth':64,'MaxExpressionNodes':100000,'MaxResolveSteps':1000000,'MaxReferenceDepth':64}
write('resource-policy.json',dict(version=1,zero='bounded default',negative='configuration error',workers={'default':5,'min':1,'max':64},defaults=defaults,global_budgets=['MaxFiles','MaxCandidateFiles','MaxTotalReadBytes','MaxArchiveEntries','MaxExpandedBytes','MaxComponents','MaxObservations','MaxEdges'],per_parser=['MaxFieldBytes','MaxSyntaxDepth','MaxExpressionNodes','MaxResolveSteps','MaxReferenceDepth','MaxArchiveDepth'],additional_hard_caps={'structured_text_bytes':16<<20,'gomod_directives':200000,'RPM_header_entries':65536},on_shared_budget_exhaustion='discard concurrently produced graph; retain deterministic discovery diagnostics',policy_basis='Versioned bounded defaults; not claimed as production sizing evidence',strict_preallocation_status='File/archive/structured syntax readers are bounded before expansion. Component/edge output limits also apply during report insertion; some format DTO slices are constructed under syntax/file bounds before output-count validation.'))
# Copy licenses and NOTICE from pinned references, separate from runtime code.
for name in ['x-mod','go-rpmdb','toml','yaml','go-dep-parser','cyclonedx-go']:
 for lic in ['LICENSE','COPYING','NOTICE']:
  p=ref/name/lic
  if p.is_file():dest=sca/'licenses'/name/lic;dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(p,dest)
# Fixture manifests use a content hash match where possible, otherwise exact baseline
# or the explicit independently captured RPM oracle relationship.
source_hashes={}
for name in ['go-dep-parser','toml','go-rpmdb','x-mod','cyclonedx-go']:
 for p in (ref/name).rglob('*'):
  if p.is_file() and '.git' not in p.parts and p.stat().st_size<50<<20:source_hashes.setdefault(sha(p),[]).append(name+'/'+str(p.relative_to(ref/name)))
# ZIP storage preserves the reviewed per-file provenance; do not replace it
# with hashes of archive containers or rediscover a now-expanded checkout.
from fixture_store import FixtureStore
fixture_store = FixtureStore(sca)
fixture_store.verify()
manifest = list(fixture_store.rows.values())
# Current production symbols and file-level provenance. Symbols remain tied to exact hashes.
inv=json.loads(subprocess.check_output(['go','run','scripts/sca/source_audit.go'],text=True,encoding='utf-8'));entries=[]
lock=json.loads((sca/'upstream-sources.lock.json').read_text())
for f in inv['files']:
 if f['test'] or '/internal/testcheck/' in f['path']:continue
 rel=f['path'].removeprefix('common/sca/');up=[];strategy='REWRITE';reason='Owned bounded implementation or compatibility conversion; no old runtime fallback.'
 if rel.startswith('analyzer/dep-parser/'):
  u=ref/'go-dep-parser'/'pkg'/rel.removeprefix('analyzer/dep-parser/');up=[u] if u.exists() else []
  if not up and '/java/gradle/' in rel:u=ref/'go-dep-parser/pkg/gradle/lockfile'/Path(rel).name;up=[u] if u.exists() else []
  strategy='TRIM_AND_REWRITE';reason='Retained required record helpers only; JSON/TOML/YAML/runtime/IO framework replaced. Source/reference identity and diagnostics changes recorded in README.md.'
 elif rel=='analyzer/parser_java_jar.go':
  up=[ref/'go-dep-parser/pkg/java/jar/parse.go'];strategy='EXTRACT_AND_REWRITE';reason='Remove Sonatype client, filename inference, temporary files and optional offline mode. Bounded ZIP and explicit metadata only.'
 elif rel.startswith('core/gomod/'):
  up=[ref/'x-mod/modfile/read.go',ref/'x-mod/modfile/rule.go',ref/'x-mod/semver/semver.go'];strategy='EXTRACT_AND_REWRITE';reason='Retain lexer and module/version validation concepts; remove edit/print/work-file APIs, download resolution, and public x/mod types.'
 elif rel.startswith('core/locktoml/'):
  up=[ref/'toml'/n for n in ['lex.go','parse.go','error.go']];strategy='EXTRACT_AND_TRIM';reason='Finite lock lexer/scalars/table parser; remove encoder, reflection binding, environment options, dates, TOML 1.1.'
 elif rel.startswith('core/rpm/'):
  up=[ref/'go-rpmdb'/n for n in ['pkg/entry.go','pkg/bdb/bdb.go','pkg/ndb/ndb.go']];reason='Binary layout reference and owned bounded readers; no SQL driver or generic database API.'
 elif rel.startswith('dxtypes/') and ('bom' in rel):up=[ref/'cyclonedx-go/cyclonedx.go'];reason='Own fixed CycloneDX 1.5 DTO; reference schema shape, no SDK runtime.'
 previous=subprocess.run(['git','show',baseline+':'+f['path']],capture_output=True).stdout
 entries.append(dict(local_file=f['path'],sha256=f['sha256'],symbols=f['symbols'] or [],decision=strategy,reason=reason,yak_baseline_sha256=hashlib.sha256(previous).hexdigest() if previous else None,upstream=[dict(path=str(p.relative_to(ref)),sha256=sha(p)) for p in up if p.exists()],maintenance_owner='yaklang/yaklang SCA maintainers',review='implementation self-review; pinned upstream changes require re-evaluation',tests=[str(p.relative_to(sca)) for p in (sca/rel).parent.glob('*test.go')]))
write('source-extraction-map.json',dict(schema_version=1,baseline=baseline,entries=entries,removed_capabilities=['x/mod editing and printing','TOML reflection decoder and encoder','general YAML runtime','go-rpmdb database/sql driver and host Open','CycloneDX SDK runtime','Docker and Git acquisition','all-pairs name-based graph merge','aggregate common/utils common/log common/filter runtime edges'],reference_only=['yaml scanner/decoder','SQLite driver module snapshot']))
def gomod_mod_tests(case, line):
 mapping={
  'TestParse':['core/gomod/upstream_test.go:TestUpstreamGoModFixtures'],
  'without go version':['core/gomod/upstream_test.go:TestUpstreamGoModFixtures/no-go-version'],
  'replace':['core/gomod/upstream_test.go:TestUpstreamGoModFixtures/replaced'],
  'no replace':['core/gomod/upstream_test.go:TestUpstreamGoModFixtures/no-replace'],
  'replace with version':['core/gomod/upstream_test.go:TestUpstreamGoModFixtures/replaced-with-version'],
  'replaced with version mismatch':['core/gomod/upstream_test.go:TestUpstreamGoModFixtures/replaced-with-version-mismatch'],
  'replaced with local path':['core/gomod/upstream_test.go:TestUpstreamGoModFixtures/replaced-with-local-path'],
  'replaced with local path and version':['core/gomod/upstream_test.go:TestUpstreamGoModFixtures/replaced-with-local-path-and-version'],
  'replaced with local path and version, mismatch':['core/gomod/upstream_test.go:TestUpstreamGoModFixtures/replaced-with-local-path-and-version-mismatch'],
  'go 1.16':['core/gomod/upstream_test.go:TestUpstreamGoModFixtures/go116'],
  'TestModuleID':['core/gomod/upstream_test.go:TestModuleID'],
  'github.com/aquasecurity/trivy':['core/gomod/upstream_test.go:TestModuleID/github.com/aquasecurity/trivy'],
  'pseudo version':['core/gomod/upstream_test.go:TestModuleID/pseudo-version'],
  'github.com/aquasecurity/go-dep-parser':['core/gomod/upstream_test.go:TestModuleID/github.com/aquasecurity/go-dep-parser'],
 }
 if case=='normal':
  return ['core/gomod/upstream_test.go:TestUpstreamGoModFixtures/normal'] if line<100 else ['core/gomod/upstream_test.go:TestModuleID/normal']
 return mapping.get(case)
# Every upstream table label is an individual candidate, along with named tests.
# Disposal is per selected format, with exact original file/line/hash preserved.
candidates=[]
for p in sorted((ref/'go-dep-parser/pkg').rglob('*.go')):
 if 'test' not in p.name or 'testdata' in p.parts:continue
 r=str(p.relative_to(ref/'go-dep-parser/pkg'));localdir=sca/'analyzer/dep-parser'/Path(r).parent
 if r.startswith('java/jar/'):
  localdir=sca/'analyzer'
 if r.startswith('gradle/lockfile/'):localdir=sca/'analyzer/dep-parser/java/gradle'
 selected=localdir.exists() or r.startswith(('golang/mod/','golang/sum/'))
 if not selected:continue
 localtests=[str(x.relative_to(sca)) for x in localdir.glob('*test.go')] if localdir.exists() else ['core/gomod/upstream_test.go','core/gomod/parse_test.go']
 src=p.read_text();labels=[]
 for lineno,line in enumerate(src.splitlines(),1):
  m=re.search(r'func (Test\w+)\(',line) or re.search(r'\bname:\s*"([^"]+)"',line)
  if m:labels.append((lineno,m.group(1)))
 for line,name in labels:
  status='ADAPTED';reason='Same pinned fixtures and expected fields; own standard-library assertion harness and filesystem. New metadata is independently tested.'
  mapped=list(localtests)
  if r.startswith('golang/mod/'):
   mapped=gomod_mod_tests(name, line) or []
   reason='Pinned go.mod fixture declarations (replace/local path/version mismatch/Go 1.16 indirect) asserted in core/gomod; not a dump of unrelated *_test.go files.'
  if r.startswith('golang/sum/'):
   status='SEMANTIC_REPLACEMENT';reason='go.sum is checksum evidence, never an inventory; core/gomod ParseSum validates exact path/version/mod key. See README.md.'
   mapped=['core/gomod/parse_test.go:TestParseSumEvidence','core/gomod/parse_test.go:TestSumIdentityConflict','report_contract_test.go:TestUnknownManifestVersionAndSumEvidence']
  if r.startswith('java/jar/'):
   mapped=['analyzer/upstream_jar_test.go'];reason='Static jar material migrated; remote SHA1/artifact lookup removed, inferred Manifest fields independently checked.'
  if r.startswith('java/pom/'):
   reason='HTTP/cache fixtures replaced by explicit readonly repository tree; coordinate mismatch/range/environment behavior is asserted in the mapped tests.'
  candidates.append(dict(source='go-dep-parser/'+r,source_sha256=sha(p),line=line,case=name,disposition=status,local_tests=mapped,reason=reason))
# Grammar fixture candidates are individually enumerable and executable.
for f in manifest:
 if f['path'].startswith('core/locktoml/testdata/upstream/') and f['path'].endswith('.toml'):
  rel=f['path'].split('testdata/upstream/',1)[1]
  reference_only=rel.startswith(('valid-next/','invalid-next/'))
  negative=rel.startswith('invalid/')
  next_only=rel in ['valid/string/escape-esc.toml','valid/string/hex-escape.toml','valid/inline-table/newline.toml','valid/key/unicode.toml','valid/datetime/no-seconds.toml']
  def has_date(v):
   if isinstance(v,dict):
    return isinstance(v.get('type'),str) and any(x in v['type'] for x in ['date','time']) or any(has_date(x) for x in v.values())
   return isinstance(v,list) and any(has_date(x) for x in v)
  oracle=(sca/f['path']).with_suffix('.json')
  date=oracle.exists() and has_date(json.loads(oracle.read_text()))
  disposition='OUT_OF_SCOPE' if reference_only else ('SEMANTIC_REPLACEMENT' if next_only or date else 'MIGRATED_AS_IS')
  reason='Reference-only TOML 1.1 corpus; not executed and not counted as migrated' if reference_only else ('Explicit unsupported-syntax negative; grammar excludes dates and TOML 1.1' if next_only or date else ('Pinned invalid-syntax rejection' if negative else 'Exact typed upstream JSON oracle'))
  candidates.append(dict(source=f['path'],source_sha256=f['sha256'],case=f['path'],disposition=disposition,local_tests=[] if reference_only else ['core/locktoml/read_test.go:TestUpstreamSyntaxCorpus/'+rel],reason=reason))
for name in ['x-mod','go-rpmdb','toml','yaml','cyclonedx-go']:
 dirs={'x-mod':[ref/name/'modfile'],'go-rpmdb':[ref/name/'pkg'],'toml':[ref/name],'yaml':[ref/name],'cyclonedx-go':[ref/name]}[name]
 for d in dirs:
  for p in sorted(d.glob('*test.go')):
   for line,text in enumerate(p.read_text().splitlines(),1):
    m=re.match(r'func (?:\([^)]*\)\s+)?(Test\w+)\(',text)
    if not m:continue
    test=m.group(1);status='OUT_OF_SCOPE';why='Generic library API beyond frozen SCA contract; encoder/editor/Go reflection/custom callback/SDK helper is not retained.';dest=[]
    if name=='x-mod' and test in ['TestParsePunctuation','TestParseVersions','TestComments','TestModulePath']:
     status='SEMANTIC_REPLACEMENT';why='Readonly bounded parser tests preserve relevant token, version, module, and indirect-comment semantics; no pretty-printer contract.';dest=['core/gomod/upstream_test.go','core/gomod/parse_test.go']
    if name=='go-rpmdb' and test in ['TestPackageList','TestRpmDB_Package','Test_headerImport']:
     status='SEMANTIC_REPLACEMENT';why='All 16 database inventories compared to isolated pinned reader, plus rpm-qa expectations and bounded Header fuzz.';dest=['core/rpm/read_test.go']
    if name=='toml' and test in ['TestToml','TestTomlNextFails','TestErrorPosition','TestParseError']:
     status='SEMANTIC_REPLACEMENT';why='Complete relevant static TOML syntax corpus plus bounded syntax errors; no upstream error-object/formatting API promise.';dest=['core/locktoml/read_test.go']
    if name=='yaml' and test in ['TestUnmarshal','TestUnmarshalErrors']:
     status='SEMANTIC_REPLACEMENT';why='Only pnpm v5/v6 finite grammar; upstream reflection/aliases/tags excluded; six full pnpm fixtures and explicit unsupported negatives.';dest=['core/lockyaml/read_test.go','analyzer/dep-parser/nodejs/pnpm/upstream_test.go']
    candidates.append(dict(source=name+'/'+str(p.relative_to(ref/name)),source_sha256=sha(p),line=line,case=test,disposition=status,local_tests=dest,reason=why))
# Retain old Yak test provenance from git, including explicitly removed acquisition.
for name in subprocess.check_output(['git','ls-tree','-r','--name-only',baseline,'common/sca'],text=True).splitlines():
 if not name.endswith('_test.go'):continue
 raw=subprocess.check_output(['git','show',baseline+':'+name]);src=raw.decode();local=root/name
 for lineno,line in enumerate(src.splitlines(),1):
  m=re.match(r'func (Test\w+)\(',line)
  if not m:continue
  symbol=m.group(1);removed=any(x in symbol.lower() for x in ['docker','image','container','gitrepo'])
  candidates.append(dict(source='yak@'+baseline+':'+name,source_sha256=hashlib.sha256(raw).hexdigest(),line=lineno,case=symbol,disposition='REMOVED_SCOPE' if removed else 'ADAPTED',local_tests=['removed_scope_test.go'] if removed else ([str(local.relative_to(sca))] if local.exists() else ['scan_contract_test.go','model/report_test.go']),reason='Explicit acquisition removal' if removed else 'Frozen inventory and independent graph-evidence contract; intentional golden changes are asserted in the mapped tests. Original expectation preserved at baseline git object.'))
write('test-migration-map.json',dict(schema_version=1,baseline=baseline,candidates=candidates,coverage_claim='Candidate disposition inventory; execution and independent assertions are separate gates. Do not infer test coverage from the number of rows.',original_yak_archive={'sha256':sha(evidence/'baseline.tar'),'git_commit':baseline},excluded_producer_tools='Docker/package managers/rpm CLI only generated pinned materials upstream, never test/runtime dependencies'))
print('contracts',len(contracts),'source files',len(entries),'fixtures',len(manifest),'test candidates',len(candidates))
