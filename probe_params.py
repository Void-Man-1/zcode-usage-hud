"""Probe billing/balance parameter variants with the zcode JWT."""
import json, hashlib, base64, os, urllib.request, urllib.error
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

CREDS = os.path.join(os.environ['USERPROFILE'], '.zcode', 'v2', 'credentials.json')
creds = json.load(open(CREDS, encoding='utf-8'))
home = os.path.expanduser('~').rstrip('\\')
secret = 'zcode-credential-fallback:win32:' + home + ':' + os.environ.get('USERNAME', '')
key = hashlib.sha256(secret.encode()).digest()

def dec(s):
    if not s.startswith('enc:v1:'):
        return s
    parts = s[len('enc:v1:'):].split('.')
    iv, tag, ct = [base64.urlsafe_b64decode(p + '=' * (-len(p) % 4)) for p in parts]
    return AESGCM(key).decrypt(iv, ct + tag, None).decode('utf-8')

jwt2 = dec(creds['zcodejwttoken'])

def probe(url, headers=None):
    h = {'Authorization': 'Bearer ' + jwt2}
    h.update(headers or {})
    req = urllib.request.Request(url, headers=h)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            return r.status, r.read().decode()[:300]
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode('utf-8', 'replace')[:300]
    except Exception as e:
        return 'ERR', type(e).__name__ + ' ' + str(e)

BASE = 'https://zcode.z.ai/api/v1/zcode-plan/billing'
tests = [
    BASE + '/balance',
    BASE + '/balance?app_version=3.11.2',
    BASE + '/balance?app_version=0.16.5',
    BASE + '/balance?app_version=4.0.0',
    BASE + '/current',
    BASE + '/current?app_version=3.11.2',
]
for u in tests:
    s, b = probe(u)
    print(s, u)
    print('   ', b.replace('\n', ' ')[:280])
