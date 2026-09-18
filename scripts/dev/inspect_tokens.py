"""Inspect decrypted token claims + probe auth variants for the balance API."""
import json, hashlib, base64, os, time, urllib.request, urllib.error
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

def jwt_claims(tok):
    try:
        p = tok.split('.')[1]
        p += '=' * (-len(p) % 4)
        return json.loads(base64.urlsafe_b64decode(p))
    except Exception as e:
        return {'err': str(e)}

def show_claims(name, tok):
    c = jwt_claims(tok)
    red = {}
    for k, v in c.items():
        if isinstance(v, str) and len(v) > 60:
            red[k] = v[:24] + '...<%d>' % len(v)
        else:
            red[k] = v
    print('==', name, 'len', len(tok))
    print(json.dumps(red, indent=1, default=str))
    if 'exp' in c:
        print('exp:', time.strftime('%Y-%m-%d %H:%M:%S', time.localtime(c['exp'])),
              'expired:', c['exp'] < time.time())
    print()

access = dec(creds['oauth:zai:access_token'])
jwt2 = dec(creds['zcodejwttoken'])
userinfo = json.loads(dec(creds['oauth:zai:user_info']))
print('== user_info (redacted)')
def redact(o):
    if isinstance(o, dict):
        return {k: redact(v) for k, v in o.items()}
    if isinstance(o, str) and len(o) > 40:
        return o[:20] + '...<%d>' % len(o)
    return o
print(json.dumps(redact(userinfo), indent=1)[:2000])
print()
show_claims('access_token', access)
show_claims('zcodejwttoken', jwt2)

URL = 'https://zcode.z.ai/api/v1/zcode-plan/billing/balance?app_version=3.11.2'
variants = [
    ('access Bearer plain', {'Authorization': 'Bearer ' + access}),
    ('access Bearer + UA', {'Authorization': 'Bearer ' + access,
                            'User-Agent': 'zcode/3.11.2'}),
    ('access raw', {'Authorization': access}),
    ('jwt2 Bearer', {'Authorization': 'Bearer ' + jwt2}),
]
for name, headers in variants:
    req = urllib.request.Request(URL, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            body = r.read().decode()
            print(name, '-> HTTP', r.status, body[:120])
            break
    except urllib.error.HTTPError as e:
        print(name, '-> HTTP', e.code, e.read()[:160])
    except Exception as e:
        print(name, '-> ERROR', type(e).__name__, e)
