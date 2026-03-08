#!/usr/bin/env python3
import json
import os
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
import urllib.error


def pick_port():
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return port


def http_json(method, url, body=None, headers=None, timeout=5):
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
    h = {"Content-Type": "application/json"}
    if headers:
        h.update(headers)
    req = urllib.request.Request(url, data=data, headers=h, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8")
            parsed = json.loads(raw) if raw else {}
            return resp.status, parsed, raw
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8")
        return e.code, None, raw
    except urllib.error.URLError:
        return 0, None, ""


def wait_for_up(base_url, timeout=45):
    end = time.time() + timeout
    while time.time() < end:
        code, _, _ = http_json("GET", base_url + "/v1/meta/capabilities")
        if code in (200, 401):
            return True
        time.sleep(0.2)
    return False


def main():
    out_path = sys.argv[1] if len(sys.argv) > 1 else ""
    port = pick_port()
    operator_token = "op_test_token"
    cfg = {
        "security_profile": "dev",
        "address": f"127.0.0.1:{port}",
        "audit_path": os.path.join(tempfile.gettempdir(), f"promptlock-redteam-{port}.jsonl"),
        "policy": {
            "default_ttl_minutes": 5,
            "min_ttl_minutes": 1,
            "max_ttl_minutes": 30,
            "max_secrets_per_request": 5,
        },
        "auth": {
            "enable_auth": True,
            "operator_token": operator_token,
            "allow_plaintext_secret_return": False,
            "session_ttl_minutes": 10,
            "grant_idle_timeout_minutes": 120,
            "grant_absolute_max_minutes": 240,
            "bootstrap_token_ttl_seconds": 60,
            "cleanup_interval_seconds": 60,
            "rate_limit_window_seconds": 60,
            "rate_limit_max_attempts": 100,
        },
        "execution_policy": {
            "allowlist_prefixes": ["curl", "go", "python", "git", "npm", "node", "make", "pytest"],
            "denylist_substrings": ["printenv", "/proc/", "environ"],
            "output_security_mode": "none",
            "max_output_bytes": 32768,
            "default_timeout_sec": 10,
            "max_timeout_sec": 30,
        },
        "network_egress_policy": {
            "enabled": True,
            "require_intent_match": True,
            "allow_domains": ["api.github.com"],
            "intent_allow_domains": {"run_tests": ["api.github.com"]},
            "deny_substrings": ["169.254.169.254", "metadata.google.internal", "localhost", "127.0.0.1"],
        },
        "secrets": [{"name": "github_token", "value": "demo"}],
        "intents": {"run_tests": ["github_token"]},
    }

    with tempfile.TemporaryDirectory(prefix="promptlock-redteam-") as td:
        cfg_path = os.path.join(td, "config.json")
        with open(cfg_path, "w", encoding="utf-8") as f:
            json.dump(cfg, f)

        env = os.environ.copy()
        env["PROMPTLOCK_CONFIG"] = cfg_path
        repo = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
        bin_path = os.path.join(td, "promptlockd-redteam")
        build = subprocess.run(["go", "build", "-o", bin_path, "./cmd/promptlockd"], cwd=repo, env=env)
        if build.returncode != 0:
            report = {"ok": False, "results": [{"name": "broker_build", "ok": False, "detail": "go build failed"}]}
            print(json.dumps(report, indent=2))
            if out_path:
                with open(out_path, "w", encoding="utf-8") as f:
                    json.dump(report, f, indent=2)
            return 1

        log_path = os.path.join(td, "broker.log")
        logf = open(log_path, "w", encoding="utf-8")
        proc = subprocess.Popen([bin_path], cwd=repo, env=env, stdout=logf, stderr=logf)

        results = []
        base = f"http://127.0.0.1:{port}"
        try:
            if not wait_for_up(base):
                logf.flush()
                snippet = ""
                try:
                    with open(log_path, "r", encoding="utf-8", errors="ignore") as lf:
                        snippet = "\n".join(lf.read().splitlines()[-20:])
                except Exception:
                    snippet = ""
                results.append({"name": "broker_start", "ok": False, "detail": "broker did not start in time", "log_tail": snippet})
                report = {"ok": False, "results": results}
                print(json.dumps(report, indent=2))
                if out_path:
                    with open(out_path, "w", encoding="utf-8") as f:
                        json.dump(report, f, indent=2)
                return 1

            code, _, _ = http_json("POST", base + "/v1/leases/approve?request_id=x")
            results.append({"name": "auth_bypass_operator_endpoint", "ok": code == 401, "status": code, "expected": 401})

            op_h = {"Authorization": f"Bearer {operator_token}"}
            code, body, _ = http_json("POST", base + "/v1/auth/bootstrap/create", {"agent_id": "a1", "container_id": "c1"}, op_h)
            ok_boot = code == 200 and body and "bootstrap_token" in body
            results.append({"name": "bootstrap_create", "ok": ok_boot, "status": code})
            if not ok_boot:
                raise RuntimeError("bootstrap create failed")

            boot = body["bootstrap_token"]
            code, body, _ = http_json("POST", base + "/v1/auth/pair/complete", {"token": boot, "container_id": "c1"})
            ok_pair = code == 200 and body and "grant_id" in body
            results.append({"name": "pair_complete", "ok": ok_pair, "status": code})
            if not ok_pair:
                raise RuntimeError("pair complete failed")
            grant = body["grant_id"]

            code, _, _ = http_json("POST", base + "/v1/auth/pair/complete", {"token": boot, "container_id": "c1"})
            results.append({"name": "bootstrap_replay_denied", "ok": code == 403, "status": code, "expected": 403})

            code, body, _ = http_json("POST", base + "/v1/auth/session/mint", {"grant_id": grant})
            ok_sess = code == 200 and body and "session_token" in body
            results.append({"name": "session_mint", "ok": ok_sess, "status": code})
            if not ok_sess:
                raise RuntimeError("session mint failed")
            sess = body["session_token"]

            agent_h = {"Authorization": f"Bearer {sess}"}
            code, _, _ = http_json("POST", base + "/v1/leases/approve?request_id=x", {}, agent_h)
            results.append({"name": "role_confusion_agent_on_operator", "ok": code == 401, "status": code, "expected": 401})

            payload = {
                "lease_token": "fake",
                "intent": "run_tests",
                "command": ["curl", "http://169.254.169.254/latest/meta-data"],
                "secrets": ["github_token"],
                "command_fingerprint": "fp",
                "workdir_fingerprint": "wd",
            }
            code, _, _ = http_json("POST", base + "/v1/leases/execute", payload, agent_h)
            results.append({"name": "egress_bypass_denied", "ok": code == 403, "status": code, "expected": 403})

        except Exception as exc:
            results.append({"name": "harness_exception", "ok": False, "detail": str(exc)})
        finally:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
            logf.close()

        ok = all(r.get("ok", False) for r in results)
        report = {"ok": ok, "results": results}
        rendered = json.dumps(report, indent=2)
        print(rendered)
        if out_path:
            with open(out_path, "w", encoding="utf-8") as f:
                f.write(rendered + "\n")
        return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
