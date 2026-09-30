import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec=importlib.util.spec_from_file_location('quality',Path(__file__).with_name('check_go_test_json.py'))
quality=importlib.util.module_from_spec(spec);spec.loader.exec_module(quality)
class RequiredTestGate(unittest.TestCase):
    def events(self, rows):
        return '\n'.join(json.dumps(dict(Package=p,Action=a,**({'Test':t} if t else {}))) for p,a,t in rows)
    def test_real_pass(self):
        self.assertTrue(quality.check(self.events([('p','run','TestOne'),('p','pass','TestOne'),('p','pass','')]),[('p','TestOne')])['ok'])
    def test_empty_and_missing(self):
        self.assertFalse(quality.check('',[('p','TestOne')])['ok'])
        self.assertFalse(quality.check('',[])['ok'])
    def test_duplicate_name_wrong_package(self):
        self.assertFalse(quality.check(self.events([('q','run','TestOne'),('q','pass','TestOne'),('q','pass','')]),[('p','TestOne')])['ok'])
    def test_skip_required_subtest(self):
        self.assertFalse(quality.check(self.events([('p','run','TestOne'),('p','skip','TestOne/case'),('p','pass','TestOne'),('p','pass','')]),[('p','TestOne')])['ok'])
    def test_unrelated_skip_is_not_failure(self):
        self.assertTrue(quality.check(self.events([('p','skip','TestExternal'),('p','run','TestOne'),('p','pass','TestOne'),('p','pass','')]),[('p','TestOne')])['ok'])
    def test_package_fail_and_no_run(self):
        self.assertFalse(quality.check(self.events([('p','run','TestOne'),('p','pass','TestOne'),('p','fail','')]),[('p','TestOne')])['ok'])
        self.assertFalse(quality.check(self.events([('p','pass','TestOne'),('p','pass','')]),[('p','TestOne')])['ok'])
    def test_bad_json(self):
        self.assertFalse(quality.check('compiler noise',[('p','TestOne')])['ok'])
    def test_legacy_requires_same_package_terminal_pass(self):
        self.assertFalse(quality.check(self.events([('p','run','TestOne'),('p','pass','TestOne')]),[(None,'TestOne')])['ok'])
        self.assertFalse(quality.check(self.events([('p','run','TestOne'),('q','pass','TestOne'),('q','pass','')]),[(None,'TestOne')])['ok'])
    def test_concatenated_runs_cannot_borrow_terminal_pass(self):
        complete=[('p','start',''),('p','run','FuzzOne'),('p','pass','FuzzOne'),('p','pass','')]
        partial=[('p','start',''),('p','run','FuzzTwo'),('p','pass','FuzzTwo')]
        required=[('p','FuzzOne'),('p','FuzzTwo')]
        self.assertFalse(quality.check(self.events(complete+partial),required)['ok'])
        self.assertTrue(quality.check(self.events(complete+partial+[('p','pass','')]),required)['ok'])
        self.assertFalse(quality.check(self.events(complete+partial[1:]),required)['ok'])
    def test_inventory_duplicate(self):
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'inventory.json'
            entry={'package':'p','test':'TestOne','tiers':['full']}
            path.write_text(json.dumps({'tests':[entry,entry]}))
            with self.assertRaises(ValueError):quality.inventory(path,'full')
