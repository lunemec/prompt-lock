# Hardened mTLS Setup (Canonical Flow)

This is the canonical setup for hardened profile over TCP with mTLS.

## 1) Generate / provision certificates
- server certificate + key for PromptLock listener
- client CA certificate used to verify client certificates
- client certificate(s) signed by the client CA

## 2) Configure PromptLock

```json
{
  "security_profile": "hardened",
  "address": "0.0.0.0:8765",
  "unix_socket": "",
  "tls": {
    "enable": true,
    "cert_file": "/etc/promptlock/tls/server.crt",
    "key_file": "/etc/promptlock/tls/server.key",
    "client_ca_file": "/etc/promptlock/tls/clients-ca.crt",
    "require_client_cert": true
  },
  "auth": {
    "enable_auth": true,
    "operator_token": "REPLACE_ME",
    "allow_plaintext_secret_return": false
  }
}
```

Notes:
- `unix_socket` must be empty when using TCP mTLS listener.
- startup fails fast if cert/key/CA config is incomplete.

## 3) Validate behavior
- client without certificate: TLS handshake should fail
- client with valid certificate: handshake succeeds, then endpoint auth rules apply

## 4) Operational checks
- rotate server and client certs regularly
- store keys/certs outside agent-writable mounts
- verify audit integrity after transport config changes
