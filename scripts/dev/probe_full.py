"""Full ZCode API probe with proper ZCode headers."""
import json, hashlib, base64, os, urllib.request, urllib.error, uuid, platform

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
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
    return AESGCM(key).decrypt(iv, ct + tag, None).decode('utf-8')

access = dec(creds['oauth:zai:access_token'])
jwt2 = dec(creds['zcodejwttoken'])
try:
    tel = json.load(open(os.path.join(os.environ['USERPROFILE'], '.zcode','v2','telemetry-state.json')))
    mid = tel.get('deviceMid','')
except Exception:
    mid = ''

BASE_HEADERS = {
    'User-Agent': 'ZCode/3.11.2',
    'HTTP-Referer': 'https://zcode.z.ai',
    'X-Title': 'Z Code@electron',
    'X-ZCode-App-Version': '3.11.2',
    'X-Platform': 'win32-x64',
    'X-Os-Category': 'windows',
    'X-Client-Language': 'en-US',
    'X-Client-Timezone': 'Europe/Warsaw',
    'X-Os-Version': platform.version(),
    'X-Release-Channel': 'stable',
}
if mid:
    BASE_HEADERS['X-Device-Mid'] = mid

def probe(url, token, label):
    h = dict(BASE_HEADERS)
    h['Authorization'] = 'Bearer ' + token
    h['x-request-id'] = str(uuid.uuid4())
    req = urllib.request.Request(url, headers=h)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            body = r.read()
            print(f"[{label}] HTTP {r.status} {url}")
            try:
                j = json.loads(body.decode())
                print(json.dumps(j, indent=1, ensure_ascii=False)[:6000])
            except Exception:
                print(body[:1000])
            print()
            return body
    except urllib.error.HTTPError as e:
        b = e.read()
        print(f"[{label}] HTTP {e.code} {url} -> {b[:500]}")
        print()
    except Exception as e:
        print(f"[{label}] ERR {url} -> {type(e).__name__} {e}")
        print()

TOKENS = [('access', access), ('jwt2', jwt2)]
URLS = [
    'https://zcode.z.ai/api/v1/zcode-plan/billing/balance?app_version=3.11.2',
    'https://zcode.z.ai/api/v1/zcode-plan/billing/current',
    'https://zcode.z.ai/api/v1/zcode-plan/billing/preview?app_version=3.11.2&platform=win32-x64',
    'https://zcode.z.ai/api/v1/client/configs?app_version=3.11.2&platform=win32-x64',
    'https://api.z.ai/api/biz/subscription/list',
    'https://api.z.ai/api/monitor/usage/quota/limit',
    'https://zcode.z.ai/api/v1/coding-plan/reset',
    'https://api.z.ai/api/biz/customer/getCustomerInfo',
    'https://zcode.z.ai/api/monitor/usage/quota/limit',
]
for tlabel, tok in TOKENS:
    print(f"######## TOKEN {tlabel} len={len(tok)} prefix={tok[:12]}")
    for u in URLS:
        probe(u, tok, tlabel)
