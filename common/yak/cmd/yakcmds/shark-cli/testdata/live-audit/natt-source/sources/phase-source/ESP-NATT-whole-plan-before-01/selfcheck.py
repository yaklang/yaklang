"""Pure Python constructor/refusal controls; consumes no new native output."""
from pathlib import Path
import ast,base64,copy,hashlib,json,subprocess,sys
from natt_expected import event,api_contract,same,PROTOCOL_STATS_KEYS,protocol_stats
root=Path(__file__).resolve().parent;out=root/'constructor-selfcheck01';out.mkdir(exist_ok=False)
frozen=json.loads((root/'frozen01/seven-complete-native-expectations.json').read_text(encoding='utf8'))
checks=[]
for row in frozen:
    wire=base64.b64decode(row['ESP_wire_base64'],validate=True)
    expected=event(row['baseline_native_event'],wire)
    assert same(expected,row['expected_native_event'])
    assert same(api_contract(row['baseline_native_event'],wire,row['independent_source']['time_ns']),row['expected_API_contract'])
    assert len(base64.b64decode(expected['fields']['Opaque Body']))==88
    assert expected['field_values']['values'][-2]['name']=='esp.sequence' and expected['field_values']['values'][-1]['name']=='esp.spi'
    assert 'CompletedIn' not in expected['source_bytes'] and 'Incomplete' not in expected['field_values'] and 'error' not in expected and 'fields_error' not in expected
    assert set(expected)==set(row['baseline_native_event'])-{'fields_error'}
    checks.append(dict(frame=row['frame'],complete_native_object_matches_frozen=True,extra_API_contract_matches_frozen=True))
seed={name:1000 for name in PROTOCOL_STATS_KEYS};after=protocol_stats(seed)
assert after['Messages']==1007 and after['Unknown']==993 and after['MessageBytes']==1672 and after['LimitedBytes']==1672 and after['UnclassifiedBytes']==328
for key in set(seed)-{'Messages','Unknown','MessageBytes','LimitedBytes','UnclassifiedBytes'}:assert type(after[key])is int and after[key]==seed[key]
assert not same(1,True) and not same(1,1.0) and not same({'a':1},{'a':1,'b':None})
wrong=copy.deepcopy(frozen[0]['expected_native_event']);wrong['fields']['Authentication Verified']=True
assert not same(wrong,frozen[0]['expected_native_event'])
plan=json.loads((root/'frozen01/plan.json').read_text())
before_cli=plan['baseline_CLI_root'];modules=str(Path(next(k for k in plan['source_file_bindings'] if k.endswith('public_cli_literal_continuity.py'))).parent)
args=[sys.executable,'-B','-X','utf8',str(root/'verify_whole.py'),'--kind','CLI','--plan',str(root/'frozen01/plan.json'),
    '--complete-cli-plan',str(root/'frozen01/CLI-whole-plan-template.json'),'--after',before_cli,'--after-source',plan['CLI_baseline_source'],
    '--audit-modules',modules,'--out',str(out/'partial-CLI-refusal')]
with (out/'refusal.stdout').open('xb') as stdout,(out/'refusal.stderr').open('xb') as stderr:
    result=subprocess.run(args,stdout=stdout,stderr=stderr)
assert result.returncode!=0
failure=json.loads((out/'partial-CLI-refusal/failure.json').read_text())
assert 'Incomplete CLI template cannot produce a whole-CLI certificate' in failure['failure'] and failure['no_success_certificate']is True
assert not (out/'partial-CLI-refusal/summary.json').exists()
for path in root.glob('*.py'):ast.parse(path.read_text(encoding='utf8'))
value=dict(scope=__doc__,constructor_checks=checks,stats_source_delta_exact=True,ProtocolStats_key_count=len(PROTOCOL_STATS_KEYS),
    typed_whole_comparison_negative_controls=True,partial_CLI_template_refused=True,Go_execution=False,new_native_execution=False,
    refusal_command=args,refusal_exit=result.returncode,refusal_failure_SHA256=hashlib.sha256((out/'partial-CLI-refusal/failure.json').read_bytes()).hexdigest())
(out/'summary.json').write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(summary=str(out/'summary.json'),SHA256=hashlib.sha256((out/'summary.json').read_bytes()).hexdigest(),native_objects=7,stats_keys=len(PROTOCOL_STATS_KEYS),partial_CLI_refused=True)))
