#!/usr/bin/env python3
"""#05 e2e: measure copilotInlineEdit latency and confirm mid-flight
$/cancelRequest aborts the in-flight model call through the real binary."""
import json, subprocess, time, sys, statistics

BIN = "./bin/local-copilot-nes"

def frame(o):
    b = json.dumps(o).encode()
    return b"Content-Length: %d\r\n\r\n%s" % (len(b), b)

def read_frame(f):
    length = None
    while True:
        line = f.readline()
        if not line:
            return None
        line = line.rstrip(b"\r\n")
        if line == b"":
            break
        if line.lower().startswith(b"content-length:"):
            length = int(line.split(b":")[1].strip())
    return json.loads(f.read(length))

def read_until_id(f, want):
    while True:
        m = read_frame(f)
        if m is None:
            raise RuntimeError("EOF waiting for %s" % want)
        if m.get("id") == want:
            return m

FILE = "def handle_alice():\n    return greetings(\"Alice\")\n\ndef handle_bob():\n    return greet(\"Bob\")\n"
URI = "file:///h.py"

p = subprocess.Popen([BIN], stdin=subprocess.PIPE, stdout=subprocess.PIPE)
def send(o): p.stdin.write(frame(o)); p.stdin.flush()

send({"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}); read_until_id(p.stdout,1)
send({"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":URI,"version":1,"text":FILE}}})

def req(rid):
    return {"jsonrpc":"2.0","id":rid,"method":"textDocument/copilotInlineEdit","params":{
        "textDocument":{"uri":URI,"version":1},"position":{"line":4,"character":4},"context":{"triggerKind":2}}}

# --- latency ---
lat = []
for i in range(5):
    rid = 10+i
    t0 = time.time(); send(req(rid)); read_until_id(p.stdout, rid); lat.append((time.time()-t0)*1000)
print("latency ms  min=%.0f  median=%.0f  max=%.0f" % (min(lat), statistics.median(lat), max(lat)))

# --- mid-flight cancellation ---
cancelled = 0
for i in range(4):
    rid = 300+i
    send(req(rid))
    time.sleep(0.03)  # let it get in-flight
    send({"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":rid}})
    resp = read_until_id(p.stdout, rid)
    if resp.get("error") is not None:
        cancelled += 1
print("mid-flight cancel: %d/4 requests returned an error (aborted)" % cancelled)

p.stdin.close(); p.wait(timeout=10)
# Pass if latency is sane and at least one cancel took effect (the rest may win the race
# against a very fast local model — that's fine; transport+engine cancel is unit-proven).
ok = statistics.median(lat) < 5000 and cancelled >= 1
print("\nLATENCY+CANCEL E2E:", "PASS" if ok else "FAIL")
sys.exit(0 if ok else 1)
