#!/usr/bin/env python3
"""
D1: Does sweep-next-edit-1.5B *chain* — i.e. given a buffer where the user just
made edit-1, does it predict the analogous edit-2 at the next similar site?

Faithfully reconstructs cursortab's sweep prompt (provider/sweep/sweep.go Build):
  <|file_sep|>{path}\n{full file}\n
  <|file_sep|>{path}.diff\noriginal:\n{orig}\nupdated:\n{upd}\n   (recent_changes)
  <|file_sep|>original/{path}:{s}:{e}\n{window}\n
  <|file_sep|>current/{path}:{s}:{e}\n{window+<|cursor|>}\n
  <|file_sep|>updated/{path}:{s}:{e}\n{prefill}
stop = <|file_sep|>, <|endoftext|>   temp=0
"""
import json, urllib.request

URL = "http://127.0.0.1:8000/v1/completions"
PATH = "handlers.py"
STOP = ["<|file_sep|>", "<|endoftext|>"]

def handler(i, fn):
    # 1-based line-accurate 4-line function block
    return (f"def handler_{i}(name, value):\n"
            f"    result = {fn}(name) + name + str(value)\n"
            f"    log('handler_{i}', result, name, value)\n"
            f"    return result")

def build_file(fixed_upto):
    """handlers 0..5; indices < fixed_upto use greetings (already edited), rest use greet."""
    blocks = []
    for i in range(6):
        fn = "greetings" if i < fixed_upto else "greet"
        blocks.append(handler(i, fn))
    return "\n\n".join(blocks) + "\n"

def window_for(idx, filetext):
    """Return (window_text, start_line_1based, end_line_1based) for handler_{idx}'s 4 lines."""
    lines = filetext.split("\n")
    # each block is 4 lines + 1 blank sep => block k starts at line k*5 (0-based)
    start0 = idx * 5
    win = lines[start0:start0 + 4]
    return "\n".join(win), start0 + 1, start0 + 4

def build_prompt(filetext, window, s, e, diff_orig, diff_upd):
    p = []
    p.append(f"<|file_sep|>{PATH}\n{filetext}\n")
    if diff_orig is not None:
        p.append(f"<|file_sep|>{PATH}.diff\noriginal:\n{diff_orig}\nupdated:\n{diff_upd}\n")
    p.append(f"<|file_sep|>original/{PATH}:{s}:{e}\n{window}\n")
    # cursor at very start of the window => prefill empty, model rewrites whole window
    p.append(f"<|file_sep|>current/{PATH}:{s}:{e}\n<|cursor|>{window}\n")
    p.append(f"<|file_sep|>updated/{PATH}:{s}:{e}\n")   # prefill = "" (cursor at col 0)
    return "".join(p)

def call(prompt):
    body = json.dumps({
        "model": "sweep", "prompt": prompt, "max_tokens": 128,
        "temperature": 0, "stop": STOP,
    }).encode()
    req = urllib.request.Request(URL, data=body, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=60) as r:
        d = json.loads(r.read())
    return d["choices"][0]["text"]

def run_step(label, fixed_upto, target_idx, diff_from_idx):
    filetext = build_file(fixed_upto)
    win, s, e = window_for(target_idx, filetext)
    # recent_changes = the edit that was just made at handler_{diff_from_idx}
    d_orig = f"    result = greet(name) + name + str(value)"
    d_upd  = f"    result = greetings(name) + name + str(value)"
    prompt = build_prompt(filetext, win, s, e, d_orig, d_upd)
    out = call(prompt)
    updated_win = out
    propagated = "greetings(name)" in updated_win and "greet(name) +" not in updated_win.replace("greetings(name)","")
    print(f"\n===== {label} =====")
    print(f"context: handlers 0..{fixed_upto-1} already 'greetings'; cursor on handler_{target_idx} (still 'greet')")
    print(f"recent_changes: greet -> greetings")
    print(f"--- model 'updated' output (raw) ---\n{out.rstrip()}")
    print(f"--- verdict: {'PROPAGATED greet->greetings on handler_%d' % target_idx if propagated else 'did NOT propagate'} ---")
    return propagated

print("### STEP 1: user just fixed handler_0. Cursor on handler_1. Does model propagate?")
s1 = run_step("STEP 1", fixed_upto=1, target_idx=1, diff_from_idx=0)

print("\n\n### STEP 2 (chain): STEP-1 applied (handler_0,1 fixed). Cursor on handler_2. Does re-request advance?")
s2 = run_step("STEP 2", fixed_upto=2, target_idx=2, diff_from_idx=1)

print("\n\n==================== D1 RESULT ====================")
print(f"Step 1 (propagates at all):        {'YES' if s1 else 'NO'}")
print(f"Step 2 (chain advances to next):   {'YES' if s2 else 'NO'}")
print("Chaining materializes for free:    ", "YES" if (s1 and s2) else "NO — needs stage-splitting fallback")
