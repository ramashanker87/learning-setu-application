"""Execute the exact inline Lambda policy with synthetic DynamoDB responses."""
import os
import sys
import types
import unittest
from pathlib import Path
import yaml

class Database:
    def __init__(self, role='ORG_ADMIN', active=True, state='ACTIVE'):
        self.role, self.active, self.state = role, active, state
        self.calls = []
    def query(self, **kwargs):
        self.calls.append(kwargs)
        return {'Items': [{'pk': {'S': 'MEMBER#org'}, 'sk': {'S': 'subject'}}]}
    def get_item(self, **kwargs):
        assert kwargs['ConsistentRead']
        if kwargs['Key']['pk']['S'] == 'ORG':
            return {'Item': {'state': {'S': self.state}}}
        return {'Item': {'role': {'S': self.role}, 'active': {'BOOL': self.active}, 'orgId': {'S': 'org'}}}

class PolicyTest(unittest.TestCase):
    def run_policy(self, db, attrs=None, source='CustomMessage_ForgotPassword'):
        sys.modules['boto3'] = types.SimpleNamespace(client=lambda *a, **kw: db)
        sys.modules['botocore.config'] = types.SimpleNamespace(Config=lambda **kw: kw)
        os.environ['CONTROL_TABLE'] = 'synthetic'
        template = yaml.safe_load(Path('infrastructure/template.yml').read_text())
        code = template['Resources']['OrganizationResetPolicy']['Properties']['Code']['ZipFile']
        module = {}; exec(code, module)
        event = {'triggerSource': source, 'request': {'userAttributes': attrs if attrs is not None else {'sub': 'subject', 'email_verified': 'true'}}}
        return module['handler'](event, None)
    def test_active_admin(self):
        self.assertEqual(self.run_policy(Database())['triggerSource'], 'CustomMessage_ForgotPassword')
    def test_reject_other_roles_and_inactive_accounts(self):
        for db in [Database(role='ASSESSOR'), Database(role='VIEWER'), Database(role='COMPANY_ADMIN'), Database(active=False), Database(state='SUSPENDED')]:
            with self.assertRaisesRegex(Exception, 'recovery is unavailable'): self.run_policy(db)
    def test_managed_team_and_unverified_and_missing_identity(self):
        for attrs in [{}, {'sub': 'subject', 'email_verified': 'false'}, {'sub': 'subject', 'email_verified': 'true', 'custom:team_org': 'org'}]:
            db = Database()
            with self.assertRaises(Exception): self.run_policy(db, attrs)
            self.assertEqual(db.calls, [])
    def test_other_cognito_messages_unaffected(self):
        for source in ['CustomMessage_AdminCreateUser', 'CustomMessage_SignUp', 'CustomMessage_Authentication']:
            db = Database(); self.run_policy(db, {}, source); self.assertEqual(db.calls, [])
    def test_database_failure_is_closed(self):
        db = Database(); db.query = lambda **kw: (_ for _ in ()).throw(RuntimeError('unavailable'))
        with self.assertRaises(RuntimeError): self.run_policy(db)

unittest.main()
