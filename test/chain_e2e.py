#!/usr/bin/env python3
"""E2E chaining test against the real binary: simulate sidekick's flow — a manual
fix (didChange) then a copilotInlineEdit at the next site — and confirm the model
propagates greet->greetings via recent_changes tracked in the DocumentStore."""
import json, os, subprocess, sys, threading

_REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN = os.environ.get("NES_BIN", os.path.join(_REPO_ROOT, "bin", "local-copilot-nes"))

def frame(o):
    b = json.dumps(o).encode()
    return b"Content-Length: %d\r\n\r\n%s" % (len(b), b)

def read_frame(f):
    # read headers
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

def read_until_id(f, want_id):
    while True:
        msg = read_frame(f)
        if msg is None:
            raise RuntimeError("EOF waiting for id %s" % want_id)
        if msg.get("id") == want_id:
            return msg

HANDLERS = ["a", "b", "c"]
def build_file(fixed):
    # fixed = number of handlers already using greetings
    blocks = []
    for i, name in enumerate(HANDLERS):
        fn = "greetings" if i < fixed else "greet"
        blocks.append('def handler_%d():\n    return %s("%s")' % (i, fn, name))
    return "\n\n".join(blocks) + "\n"

# handler i's `return` line is at 0-based line index i*3 + 1
def return_line(i):
    return i * 3 + 1

p = subprocess.Popen([BIN], stdin=subprocess.PIPE, stdout=subprocess.PIPE)
def send(o): p.stdin.write(frame(o)); p.stdin.flush()

URI = "file:///handlers.py"
send({"jsonrpc":"2.0","id":1,"method":"initialize","params":{}})
read_until_id(p.stdout, 1)

# didOpen: all greet, version 1
send({"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":URI,"version":1,"text":build_file(0)}}})

results = []
version = 1
# manual fix of handler_0, then chain requests at handler_1, handler_2
for i in range(1, 3):
    # apply the fix at handler_{i-1} (manual for i=1, accepted-suggestion for i=2)
    version += 1
    send({"jsonrpc":"2.0","method":"textDocument/didChange","params":{
        "textDocument":{"uri":URI,"version":version},
        "contentChanges":[{"text":build_file(i)}]}})
    # request a suggestion at handler_i's return line
    rid = 100 + i
    send({"jsonrpc":"2.0","id":rid,"method":"textDocument/copilotInlineEdit","params":{
        "textDocument":{"uri":URI,"version":version},
        "position":{"line":return_line(i),"character":4},
        "context":{"triggerKind":2}}})
    resp = read_until_id(p.stdout, rid)
    edits = resp.get("result",{}).get("edits",[])
    text = edits[0]["text"] if edits else ""
    want = 'greetings("%s")' % HANDLERS[i]
    ok = want in text
    results.append((i, ok, want, text.strip().replace("\n","\\n")))
    print("STEP handler_%d: propagated=%s (want %s)" % (i, ok, want))

p.stdin.close()
p.wait(timeout=10)

allok = all(r[1] for r in results)
print("\nCHAINING E2E:", "PASS" if allok else "FAIL")
if not allok:
    for i, ok, want, text in results:
        if not ok:
            print("  handler_%d MISSING %s in: %s" % (i, want, text))
sys.exit(0 if allok else 1)
