#!/usr/bin/env python3
import json
import secrets
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

STATE_PATH = Path('/tmp/secret-broker-state.json')
HOST = '127.0.0.1'
PORT = 8765
MAX_TTL_MIN = 60

# Demo secrets store (replace with Vault/1Password/etc in production)
SECRETS = {
    'github_token': 'DEMO_GITHUB_TOKEN_REPLACE_ME',
    'npm_token': 'DEMO_NPM_TOKEN_REPLACE_ME',
    'openai_api_key': 'DEMO_OPENAI_KEY_REPLACE_ME'
}


def utc_now():
    return datetime.now(timezone.utc)


def iso(dt):
    return dt.isoformat()


def load_state():
    if STATE_PATH.exists():
        return json.loads(STATE_PATH.read_text())
    return {'requests': {}, 'leases': {}, 'audit': []}


def save_state(state):
    STATE_PATH.write_text(json.dumps(state, indent=2))


class Handler(BaseHTTPRequestHandler):
    def _json(self, status, payload):
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(json.dumps(payload).encode())

    def _read_json(self):
        length = int(self.headers.get('Content-Length', '0'))
        body = self.rfile.read(length) if length else b'{}'
        return json.loads(body.decode() or '{}')

    def log_message(self, *_):
        return

    def do_POST(self):
        state = load_state()
        body = self._read_json()

        if self.path == '/v1/leases/request':
            ttl = int(body.get('ttl_minutes', 0))
            secrets_req = body.get('secrets', [])
            if ttl <= 0 or not secrets_req:
                return self._json(400, {'error': 'ttl_minutes>0 and secrets required'})
            req_id = f"req_{secrets.token_hex(8)}"
            state['requests'][req_id] = {
                'request_id': req_id,
                'status': 'pending',
                'agent_id': body.get('agent_id', 'unknown'),
                'task_id': body.get('task_id', 'unknown'),
                'reason': body.get('reason', ''),
                'ttl_minutes': ttl,
                'secrets': secrets_req,
                'created_at': iso(utc_now())
            }
            state['audit'].append({'event': 'request_created', 'request_id': req_id, 'at': iso(utc_now())})
            save_state(state)
            return self._json(200, {'request_id': req_id, 'status': 'pending'})

        if self.path.startswith('/v1/leases/') and self.path.endswith('/approve'):
            req_id = self.path.split('/')[3]
            req = state['requests'].get(req_id)
            if not req:
                return self._json(404, {'error': 'request not found'})
            if req['status'] != 'pending':
                return self._json(400, {'error': f"request already {req['status']}"})
            ttl = min(int(body.get('ttl_minutes', req['ttl_minutes'])), MAX_TTL_MIN)
            lease = f"lease_{secrets.token_hex(12)}"
            expires = utc_now() + timedelta(minutes=ttl)
            req['status'] = 'approved'
            req['approved_at'] = iso(utc_now())
            state['leases'][lease] = {
                'lease_token': lease,
                'request_id': req_id,
                'agent_id': req['agent_id'],
                'task_id': req['task_id'],
                'secrets': req['secrets'],
                'expires_at': iso(expires)
            }
            state['audit'].append({'event': 'request_approved', 'request_id': req_id, 'lease_token': lease, 'at': iso(utc_now())})
            save_state(state)
            return self._json(200, {'status': 'approved', 'lease_token': lease, 'expires_at': iso(expires), 'secrets': req['secrets']})

        if self.path.startswith('/v1/leases/') and self.path.endswith('/deny'):
            req_id = self.path.split('/')[3]
            req = state['requests'].get(req_id)
            if not req:
                return self._json(404, {'error': 'request not found'})
            req['status'] = 'denied'
            req['denied_reason'] = body.get('reason', '')
            state['audit'].append({'event': 'request_denied', 'request_id': req_id, 'at': iso(utc_now())})
            save_state(state)
            return self._json(200, {'status': 'denied'})

        if self.path == '/v1/leases/access':
            lease = body.get('lease_token')
            secret = body.get('secret')
            lease_obj = state['leases'].get(lease)
            if not lease_obj:
                return self._json(403, {'error': 'invalid lease'})
            if utc_now() > datetime.fromisoformat(lease_obj['expires_at']):
                return self._json(403, {'error': 'lease expired'})
            if secret not in lease_obj['secrets']:
                return self._json(403, {'error': 'secret not allowed in lease'})
            if secret not in SECRETS:
                return self._json(404, {'error': 'unknown secret'})
            state['audit'].append({'event': 'secret_access', 'lease_token': lease, 'secret': secret, 'at': iso(utc_now())})
            save_state(state)
            return self._json(200, {'secret': secret, 'value': SECRETS[secret]})

        self._json(404, {'error': 'not found'})


if __name__ == '__main__':
    print(f"Mock Secret Lease Broker listening on http://{HOST}:{PORT}")
    HTTPServer((HOST, PORT), Handler).serve_forever()
