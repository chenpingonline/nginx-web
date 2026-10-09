"""Actual native systemd smoke test on a disposable CI runner; never production."""
import http.client
import http.cookiejar
import http.server
import json
import os
import pathlib
import socket
import ssl
import subprocess
import tempfile
import threading
import time
import urllib.request

assert os.geteuid() == 0, "Run only as root on a disposable Linux runner"
base = "http://127.0.0.1:8080"
jar = http.cookiejar.CookieJar()
client = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(jar))
csrf = ""
def request(path, method="GET", body=None):
    headers = {"Content-Type": "application/json", "X-FnProxy-Request": "1", "X-CSRF-Token": csrf}
    req = urllib.request.Request(base + path, headers=headers, method=method, data=json.dumps(body).encode() if body is not None else None)
    with client.open(req, timeout=10) as response:
        raw = response.read()
        return json.loads(raw) if "application/json" in response.headers.get("Content-Type", "") else raw
def ready():
    for _ in range(100):
        try:
            request("/healthz")
            return
        except OSError:
            time.sleep(.2)
    raise RuntimeError("systemd service did not become healthy")
def login():
    global csrf
    password = pathlib.Path("/etc/nginx-web/admin_password").read_text().strip()
    csrf = request("/api/auth/login", "POST", {"username": "admin", "password": password})["csrf_token"]
class Upstream(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200); self.end_headers(); self.wfile.write(b"LINUX_PACKAGE_OK")
    def log_message(self, *args):
        pass
upstream = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Upstream)
threading.Thread(target=upstream.serve_forever, daemon=True).start()
try:
    ready(); login()
    pid = subprocess.check_output(["systemctl", "show", "nginx-web", "--property=MainPID", "--value"], text=True).strip()
    status = dict(line.split(":", 1) for line in pathlib.Path(f"/proc/{pid}/status").read_text().splitlines() if ":" in line)
    uid = subprocess.check_output(["id", "-u", "nginx-web"], text=True).strip()
    assert uid != "0" and all(v == uid for v in status["Uid"].split())
    assert int(status["CapEff"], 16) == 1 << 10 and status["NoNewPrivs"].strip() == "1"
    with tempfile.TemporaryDirectory() as directory:
        cert, key = pathlib.Path(directory)/"cert.pem", pathlib.Path(directory)/"key.pem"
        subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=linux.test", "-addext", "subjectAltName=DNS:linux.test", "-keyout", str(key), "-out", str(cert)], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        certificate = cert.read_text()
        cert_id = request("/api/certificates", "POST", {"name":"Linux CI", "certificate":certificate, "private_key":key.read_text()})["id"]
    common = {"enabled": True, "domains": ["linux.test"], "upstream_scheme":"http", "upstream_host":"127.0.0.1", "upstream_port":upstream.server_port}
    rule = request("/api/rules", "POST", dict(common, name="Linux HTTP", listen_port=80))
    request("/api/rules", "POST", dict(common, name="Linux HTTPS", listen_port=443, tls=True, certificate_id=cert_id))
    request("/api/apply", "POST", {"summary":"Native Linux package CI"})
    def proxies():
        connection=http.client.HTTPConnection("127.0.0.1",80,timeout=5)
        connection.request("GET","/",headers={"Host":"linux.test"})
        response=connection.getresponse(); assert response.status==200 and response.read()==b"LINUX_PACKAGE_OK"; connection.close()
        context=ssl.create_default_context(cadata=certificate)
        with socket.create_connection(("127.0.0.1",443),timeout=5) as plain:
            with context.wrap_socket(plain,server_hostname="linux.test") as secure:
                secure.sendall(b"GET / HTTP/1.0\r\nHost: linux.test\r\n\r\n")
                result=b""
                while chunk:=secure.recv(4096): result+=chunk
                assert b"LINUX_PACKAGE_OK" in result
    proxies()
    request("/api/rules/"+rule["id"], "PUT", dict(rule,name="Retained draft"))
    subprocess.run(["systemctl","restart","nginx-web"],check=True)
    ready(); login(); proxies()
    state=request("/api/state")
    assert state["dirty"] and any(r["name"]=="Retained draft" for r in state["rules"])
    request("/api/auth/logout","POST",{})
    assert not request("/api/auth/session")["authenticated"]
    print("PASS: native systemd, non-root/capabilities, login, HTTP 80, HTTPS 443, persisted drafts and restart")
finally:
    upstream.shutdown(); upstream.server_close()
