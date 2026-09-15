"""Verify the zcode.z.ai billing balance API works with the locally stored token."""
import json, hashlib, base64, os, urllib.request, sys
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

CREDS = os.path.join(os.environ['USERPROFILE'], '.zcode', 'v2', 'credentials.json')

creds = json.load(open(CREDS, encoding='utf-8'))
home = os.path.expanduser('~').rstrip('\\')
secret = 'zcode-credential-fallback:win32:' + home + ':' + os.environ.get('USERNAME', '')
print('secret used:', secret)
key = hashlib.sha256(secret.encode()).digest()

def dec(s):
    if not s.startswith('enc:v1:'):
        return s
    parts = s[len('enc:v1:'):].split('.')
    iv, tag, ct = [base64.urlsafe_b64decode(p + '=' * (-len(p) % 4)) for p in parts]
    return AESGCM(key).decrypt(iv, ct + tag, None).decode('utf-8')

tok = dec(creds['oauth:zai:access_token'])
print('token decrypted, len', len(tok), 'prefix:', tok[:8])

req = urllib.request.Request(
    'https://zcode.z.ai/api/v1/zcode-plan/billing/balance?app_version=3.11.2',
    headers={'Authorization': 'Bearer ' + tok})
try:
    with urllib.request.urlopen(req, timeout=15) as r:
        d = json.loads(r.read().decode())
        data = d.get('data', d)
        print('HTTP', r.status, 'code:', d.get('code'))
        for p in data.get('plans', []):
            print('PLAN:', p['name'], p['status'], 'ends_at:', p['ends_at'])
        for b in data.get('balances', []):
            print('BUCKET:', b['show_name'], '| total', b['total_units'], '| used',
                  b['used_units'], '| remaining', b['remaining_units'],
                  '| period', b['period_start'], '->', b['period_end'])
except Exception as e:
    print('ERROR:', type(e).__name__, e)
    sys.exit(1)
