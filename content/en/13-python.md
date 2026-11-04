# Python

A thorough Python primer for DevOps/SRE. Every topic is built the same way: what it is, why it's needed, how it works, an example with the result explained, and typical mistakes. The examples target Python 3.10+ (check your version: `python3 --version`).

How to read: run the examples yourself. The quickest way is the interactive `python3` console (or `ipython`): type a line and see the result immediately. In the examples the result is shown in a comment after `# →`.

## What kind of language it is and how it differs

**Interpreted.** The code isn't compiled in advance into an executable, as in Go or C#. The CPython interpreter reads a `.py` file, translates it into bytecode (intermediate instructions, cached in `__pycache__`) and executes it right away. The upsides: no build step, you can edit a script and run it immediately. The downsides: an error in a rarely called branch of code only surfaces when execution reaches it, not at build time.

**Dynamic typing.** The type belongs to the *value*, not to the *variable*. A variable is just a name (a label) that can be re-attached to a value of any type:
```python
x = 10        # x points to a number
x = "ten"     # now x points to a string — this is allowed
```
In C# you can't do that: `int x = 10; x = "ten";` won't compile.

**Strong typing.** At the same time Python does *not* silently convert types into each other, unlike JavaScript or Bash:
```python
"5" + 5        # → TypeError: can only concatenate str (not "int") to str
"5" + str(5)   # → '55'   — explicitly converted the number to a string
int("5") + 5   # → 10     — explicitly converted the string to a number
```
Why: implicit conversions breed subtle bugs (in JS `"5" - 2` equals 3, while `"5" + 2` equals `"52"`). Python makes you say explicitly what you mean.

**Everything is an object.** Numbers, strings, functions, classes, modules — all have a type, attributes and methods. Even a number: `(5).bit_length()` → 3. A function can be put into a variable, passed to another function or returned from a function. Decorators and callbacks are built on this.

**Indentation is part of the syntax.** Instead of curly braces `{}`, code blocks are marked by indentation (the standard is 4 spaces). It's not a style but a language rule: wrong indentation means an `IndentationError` or, worse, code that runs in the wrong block.
```python
if True:
    print("inside if")      # 4 spaces — the body of the if
print("after if")           # no indentation — already outside the if
```

**Philosophy.** Type `import this` in the console and "The Zen of Python" is printed. The main ideas: explicit is better than implicit, simple is better than complex, readability counts. In Python teams this is genuinely used as an argument in code review.

### 🧪 Mini task

**Task.** Check in practice that Python typing is dynamic (one variable can hold values of different types) but strong (`"5" + 5` silently becomes neither `10` nor `"55"`). Print the variable's type before and after reassignment, catch the error from adding a string to a number, and show both explicit conversions.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
x = 10
print(type(x).__name__)
x = "ten"                     # same x, now a string — dynamic typing
print(type(x).__name__)
try:
    "5" + 5                   # strong typing: won't silently concatenate
except TypeError as e:
    print("TypeError:", e)
print(int("5") + 5, "5" + str(5))
EOF
```

Expected output:

```text
int
str
TypeError: can only concatenate str (not "int") to str
10 55
```

Breakdown: `type(x)` changes from `int` to `str` — the type belongs to the value, not to the name. Adding a string to a number raises `TypeError` instead of a "guessed" result. The last line shows that the result depends on *what* you explicitly converted: the number (`10`) or the string (`55`).

</details>

## The environment: venv, pip, uv

**The problem.** Project A needs version 2.25 of the `requests` library, project B needs 2.32. If you install packages into one shared (system) Python, they conflict. Worse, the system Python is used by the OS's own tools (in Ubuntu — `apt` scripts), and if you upgrade a library in it via `sudo pip install`, you can break the system. That's why modern distributions explicitly forbid `pip install` into the system Python (the `externally-managed-environment` error).

**The solution — a virtual environment (venv).** It's a folder inside the project containing its own copy of the interpreter (a link to it) and its own installed packages. Projects are isolated from each other.

```bash
python3 -m venv .venv            # create an environment in the .venv folder
source .venv/bin/activate        # "enter" it: now python and pip come from .venv
which python                     # → /path/to/project/.venv/bin/python
pip install requests boto3       # packages are installed only into .venv
pip freeze > requirements.txt    # record the exact versions of all packages
deactivate                       # leave the environment
```
On another machine or in CI the project is restored like this: `python3 -m venv .venv && .venv/bin/pip install -r requirements.txt`. The `.venv` folder isn't committed to git (add it to `.gitignore`); only `requirements.txt` is committed.

**Pinning versions.** If `requirements.txt` says just `requests`, half a year later a new version will be installed and the script may break. That's why versions are pinned: `requests==2.32.3`. This is called pinning, and it's exactly what `pip freeze` does.

**uv — a modern replacement.** A fast package manager (written in Rust) from Astral. It creates the venv itself, installs packages dozens of times faster than pip and maintains a lock file.
```bash
uv init mytool && cd mytool      # a new project with pyproject.toml
uv add requests boto3            # add dependencies (written to pyproject.toml and uv.lock)
uv run main.py                   # run a script in the project's environment (creates it if missing)
uvx ruff check .                 # run a tool once without installing it into the project
```

**pyproject.toml** is the modern standard project description file: name, version, dependencies, tool settings (linter, tests). It replaces the old `setup.py`, `setup.cfg` and partly `requirements.txt`.

**Quality tools:** `ruff` — a linter and formatter (finds errors and unused imports, brings the code to a single style), `mypy` — type checking (see the section on type hints), `pytest` — tests.

### 🧪 Mini task

**Task.** In a temporary folder create a virtual environment, install `pyyaml` into it, pin the versions with `pip freeze`, and prove the isolation: the module imports inside the venv but not in the system `python3`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
python3 -m venv .venv
.venv/bin/pip install -q --disable-pip-version-check pyyaml
.venv/bin/pip freeze
.venv/bin/python -c "import yaml; print('venv:', yaml.__version__)"
python3 -c "import yaml" 2>&1 | tail -1
```

Expected output:

```text
PyYAML==6.0.3
venv: 6.0.3
ModuleNotFoundError: No module named 'yaml'
```

Breakdown: `pip freeze` prints the exact version — that is the line that goes into `requirements.txt`. Your version may be newer. The last line is the import error from the system Python: the package went only into `.venv`. Note that the venv doesn't even need activating: calling `.venv/bin/python` and `.venv/bin/pip` directly is enough. That's how it is done in CI and in Dockerfiles.

</details>

## Variables and names: how it really works

This is the foundation without which many of Python's "oddities" are hard to understand.

In Python a variable is **not a box holding a value** but **a name label attached to an object** in memory. The assignment `a = b` doesn't copy data; it hangs a second label on the same object.

```python
a = [1, 2, 3]
b = a            # b is a second label on THE SAME list, there's no copy
b.append(4)
print(a)         # → [1, 2, 3, 4]   — a changed too, because it's one object
print(a is b)    # → True           — is checks: is this one and the same object?

c = a.copy()     # and this is a real copy — a new object
c.append(5)
print(a)         # → [1, 2, 3, 4]   — a didn't change
```

**`is` versus `==`:**
- `==` asks: *are the values equal?*
- `is` asks: *is this literally one and the same object in memory?*

```python
x = [1, 2]
y = [1, 2]
x == y   # → True   — the values are the same
x is y   # → False  — but these are two different lists
```
The rule: use `is` only to compare with `None` (`if value is None:`), because `None` exists as a single instance. For everything else — `==`.

### 🧪 Mini task

**Task.** First predict on paper, then check: what will `a`, `b`, `c` and the three comparisons print if `b = a`, `c = a.copy()` and then `b.append(3)`?

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
a = [1, 2]
b = a
c = a.copy()
b.append(3)
print(a, b, c)
print(a is b, a is c, a == c)
EOF
```

Expected output:

```text
[1, 2, 3] [1, 2, 3] [1, 2]
True False False
```

Breakdown: `a` and `b` are two names for one list, so the `append` through `b` is visible through `a` too, and `a is b` is `True`. `c` is a separate copy made *before* the `append`, so it only has `[1, 2]`. Hence `a is c` is `False` (different objects) and `a == c` is also `False` (the values have already diverged). Had the copy been made after the `append`, `a == c` would be `True`, while `a is c` would still be `False`.

</details>

## Basic data types

### Numbers: int and float
```python
count = 42              # int — an integer
big = 2 ** 100          # → 1267650600228229401496703205376
```
`int` in Python **doesn't overflow**: a number can be of any length, as long as memory allows. In C# or Java `int` is limited to 32 bits (about 2 billion), but here you don't need to think about it.

```python
ratio = 0.75            # float — a floating-point number (a 64-bit double)
0.1 + 0.2               # → 0.30000000000000004  (!)
0.1 + 0.2 == 0.3        # → False
```
Why: a `float` is stored in binary, and the decimal 0.1 is an infinite fraction in binary (like 1/3 in decimal). It's not a Python bug; floats work this way in every language. What to do about it:
```python
import math
math.isclose(0.1 + 0.2, 0.3)       # → True — comparison with a tolerance
round(0.1 + 0.2, 2)                # → 0.3
from decimal import Decimal
Decimal("0.1") + Decimal("0.2")    # → Decimal('0.3') — exact decimal arithmetic (money)
```

Division and operators:
```python
7 / 2     # → 3.5   — ordinary division ALWAYS gives a float
7 // 2    # → 3     — integer division (drop the fractional part, round down)
7 % 2     # → 1     — the remainder (checking for evenness: n % 2 == 0)
2 ** 10   # → 1024  — exponentiation
-7 // 2   # → -4    — careful: rounds down (towards minus infinity), not towards zero
```
For readability of large numbers you can use underscores: `timeout_ms = 30_000`.

### The bool type and "truthiness"
```python
is_prod = True
has_errors = False
```
`bool` is a subclass of `int`: `True == 1`, `False == 0`. That's why `sum([True, False, True])` → 2. It's handy for counting: `sum(1 for p in pods if p["ready"])`, or even shorter `sum(p["ready"] for p in pods)`.

**Truthiness.** You can put *any* value into `if` and `while`, and Python decides by itself whether to treat it as true or false. These count as false:
- `False`, `None`
- zero of any type: `0`, `0.0`
- empty collections: `""`, `[]`, `{}`, `()`, `set()`

Everything else is true. This lets you write idiomatically:
```python
pods = []
if not pods:                # instead of if len(pods) == 0:
    print("no pods")

name = ""
display = name or "unknown" # → 'unknown' — or returns the first truthy value
```
How `and`/`or` work: they return not `True`/`False` but one of the operands. `a or b` returns `a` if `a` is truthy, otherwise `b`. That's why `x or default` is a common way to set a default value. Careful: if `x = 0` is a valid value, `x or 10` returns 10, not 0. In such cases write it explicitly: `x if x is not None else 10`.

### None — "there is no value"
`None` means the absence of a value (the equivalent of `null`). A function without `return` returns `None`. It's always checked with `is`:
```python
def find_pod(name):
    for p in pods:
        if p == name:
            return p
    # if nothing was found, the function reaches the end and implicitly returns None

result = find_pod("api")
if result is None:
    print("not found")
```

### Strings (str)
A string is an immutable sequence of Unicode characters. Cyrillic, emoji — everything works out of the box.

**Immutability** means that string methods don't change the string but return a *new* one:
```python
s = "Hello"
s.upper()      # → 'HELLO' — returned a new string
print(s)       # → 'Hello' — the original didn't change
s = s.upper()  # to "change" it, re-attach the name to the result
```

**f-strings** are the main way to insert values into text (the `f` prefix before the quotes):
```python
service, port, cpu = "api", 8080, 0.4567
f"{service}:{port}"          # → 'api:8080'
f"CPU {cpu:.1%}"             # → 'CPU 45.7%'  — format: percentage with 1 decimal place
f"{cpu:.2f}"                 # → '0.46'       — 2 digits after the point
f"[{service:>10}]"           # → '[       api]' — right-align to a width of 10
f"[{service:<10}]"           # → '[api       ]' — left
f"{port=}"                   # → 'port=8080'  — handy for debugging: the name and the value
f"{len(service) * 2}"        # → '6'          — any expression can go inside {}
```

**Useful methods** (each returns a new string or value):
```python
line = "  ERROR: disk /dev/sda1 is full  "
line.strip()                   # → 'ERROR: disk /dev/sda1 is full' — remove whitespace at the edges
line.strip().lower()           # → 'error: disk /dev/sda1 is full' — methods can be chained
line.strip().startswith("ERROR")   # → True
"/dev/sda1" in line            # → True — substring check
line.count("s")                # → 3 — how many times 's' occurs
line.replace("ERROR", "WARN")  # replace all occurrences

"api,web,db".split(",")        # → ['api', 'web', 'db'] — split by a separator
"a  b   c".split()             # → ['a', 'b', 'c'] — without an argument: by any whitespace, empty parts dropped
", ".join(["api", "web"])      # → 'api, web' — join a list of strings with a separator

"key=value=x".split("=", 1)    # → ['key', 'value=x'] — split only on the first '='
"key=value".partition("=")     # → ('key', '=', 'value') — always 3 parts, even if there's no '='
"app.tar.gz".rsplit(".", 1)    # → ['app.tar', 'gz'] — split from the right
```
A common mistake: `", ".join([1, 2])` fails with `TypeError`, because `join` only joins strings. Convert them first: `", ".join(str(x) for x in [1, 2])`.

**Indexes and slices.** Indexes start at 0; negative ones count from the end:
```python
s = "kubernetes"
s[0]      # → 'k'      — the first character
s[-1]     # → 's'      — the last one
s[0:4]    # → 'kube'   — the slice [start:end), the end is NOT included
s[4:]     # → 'rnetes' — from 4 to the end
s[:4]     # → 'kube'   — from the start up to 4
s[::2]    # → 'kbree'  — every second character (indexes 0, 2, 4, 6, 8) (the third parameter is the step)
s[::-1]   # → 'setenrebuk' — a step of -1 reverses the string
```
Slices work the same way for strings, lists and tuples. Why the end isn't included: this way `s[:4] + s[4:]` always gives the original string, and the length of the slice `s[a:b]` equals `b - a`.

**Multi-line and raw strings:**
```python
query = """
SELECT *
FROM pods
"""                         # triple quotes — a string with line breaks

pattern = r"\d+\.\d+"       # a raw string: \ doesn't escape. Needed for regular expressions and Windows paths
```

**str and bytes.** `str` is text (characters), `bytes` is raw bytes (what actually travels over the network and sits in a file). Converting between them is encoding: `"héllo".encode("utf-8")` → bytes, `b"\xc3\xa9".decode("utf-8")` → str. You meet it when working with `subprocess` (without `text=True` the output comes as bytes), sockets, base64:
```python
import base64
base64.b64decode("cGFzc3dvcmQ=").decode()   # → 'password' — this is how a k8s Secret is decoded
```

### Type conversion
```python
int("42")        # → 42
int("42.5")      # → ValueError — a string with a point can't go straight into int
int(float("42.5"))  # → 42
float("1e3")     # → 1000.0
str(3.14)        # → '3.14'
bool("False")    # → True (!) — a non-empty string is always truthy, the text isn't analysed
list("abc")      # → ['a', 'b', 'c']
int("ff", 16)    # → 255 — from hexadecimal
type(42)         # → <class 'int'>
isinstance(42, int)          # → True — the right way to check a type
isinstance(True, int)        # → True — because bool is a subclass of int
```
The `bool("False")` trap really does come up when reading environment variables: `os.environ["DEBUG"]` is always a string. The right way: `debug = os.getenv("DEBUG", "false").lower() in ("1", "true", "yes")`.

### Mutable and immutable types — why it matters

| Immutable | Mutable |
|---|---|
| `int`, `float`, `bool`, `str`, `bytes`, `tuple`, `frozenset`, `None` | `list`, `dict`, `set`, `bytearray`, objects of your classes |

An immutable object can't be changed "in place": any operation creates a new object. A mutable one is changed in place, and **all** the names pointing to it see that change.

Where this bites:
1. **Passing to a function.** The function gets a reference to the same object. If it changes the list, the list outside changes too.
   ```python
   def add_default_tag(tags):
       tags.append("managed-by-python")   # changes the original list!

   my_tags = ["prod"]
   add_default_tag(my_tags)
   print(my_tags)   # → ['prod', 'managed-by-python']
   ```
2. **Dictionary keys.** A `dict` key (and a `set` element) can only be a *hashable* object, and hashable objects are essentially the immutable ones. `{[1, 2]: "x"}` → `TypeError: unhashable type: 'list'`. A tuple is fine: `{("us-east-2", "prod"): "vpc-123"}`.
3. **Default values in functions** (see the "Functions" section — a classic trap).

### 🧪 Mini task

**Task.** A script gets its settings from environment variables: `REPLICAS=3`, `DEBUG=False`, `CPU=0.4567`, and `TIMEOUT` is not set. Convert them to the right types: replicas — `int`, the flag — `bool`, CPU — a percentage with one decimal, timeout — default 30. Show why `bool("False")` is a trap.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
REPLICAS=3 DEBUG=False CPU=0.4567 python3 - <<'EOF'
import os

def env_bool(name, default="false"):
    return os.getenv(name, default).lower() in ("1", "true", "yes")

replicas = int(os.environ["REPLICAS"])
cpu = float(os.environ["CPU"])
print(f"{replicas=} type={type(replicas).__name__}")
print("naive bool:", bool(os.environ["DEBUG"]))   # a non-empty string is always True
print("env_bool:  ", env_bool("DEBUG"))
print(f"cpu {cpu:.1%}, timeout {os.getenv('TIMEOUT') or 30}s")
EOF
```

Expected output:

```text
replicas=3 type=int
naive bool: True
env_bool:   False
cpu 45.7%, timeout 30s
```

Breakdown: everything that comes from the environment is a string, so `int()` is mandatory. `bool("False")` gives `True`: a non-empty string is truthy, the text is not analysed. The `env_bool` function compares the string against a list of "true" values and returns an honest `False`. `{cpu:.1%}` multiplies by 100 and appends the percent sign. `os.getenv('TIMEOUT') or 30` gives 30 because an unset variable is `None`.

</details>

## Collections

Collections are containers for several values. Choosing the right one is half of good code.

### list
An ordered mutable sequence. The equivalent of `List<T>` in C#, but it can hold values of different types.
```python
pods = ["api", "web"]
pods.append("db")              # add to the end            → ['api', 'web', 'db']
pods.extend(["cache", "mq"])   # add several               → [..., 'cache', 'mq']
pods.insert(0, "lb")           # insert at an index        → ['lb', 'api', ...]
last = pods.pop()              # remove the last one and return it → 'mq'
first = pods.pop(0)            # remove by index                   → 'lb'
pods.remove("web")             # remove by VALUE (the first occurrence), ValueError if absent
"api" in pods                  # → True — membership check
pods.index("db")               # → the element's position
len(pods)                      # → the length
pods[0] = "api-v2"             # change an element
```
`append` versus `extend`: `pods.append(["x", "y"])` adds *one* element — a nested list. `pods.extend(["x", "y"])` adds *two* elements.

**Sorting:**
```python
nums = [3, 1, 2]
sorted(nums)             # → [1, 2, 3] — returns a NEW list, nums doesn't change
nums.sort()              # sorts IN PLACE, returns None
result = nums.sort()     # a common mistake: result will be None

pods = [{"name": "api", "restarts": 5}, {"name": "web", "restarts": 12}]
sorted(pods, key=lambda p: p["restarts"], reverse=True)
# key is a function that tells, for each element, WHAT to compare by
# → [{'name': 'web', 'restarts': 12}, {'name': 'api', 'restarts': 5}]
```

**Speed:** access by index and `append` are instant (O(1) — "Big O", the notation for how an operation's time grows with the data size: O(1) — independent of size, O(n) — grows in proportion to the number of elements). But `in` and `remove` walk through the whole list (O(n)): on a million elements this is noticeable. If you often need to check "is the element there" — use a `set`.

### tuple
Like a list, but **immutable**. Why, if there's a list:
- it denotes a "record" with a fixed structure: `("10.0.0.1", 22)` — a host and a port;
- it can be a dictionary key and a set element (hashable);
- it protects the data from accidental change.

```python
endpoint = ("10.0.0.1", 22)
host, port = endpoint          # unpacking: spread a tuple into variables
print(host)                    # → '10.0.0.1'

def min_max(values):
    return min(values), max(values)   # the function "returns two values" — actually one tuple
lo, hi = min_max([3, 1, 4])           # → lo=1, hi=4

first, *rest = [1, 2, 3, 4]    # the asterisk collects "everything else" → first=1, rest=[2, 3, 4]
a, b = b, a                    # swap values without a temporary variable
single = ("only",)             # a one-element tuple needs a comma! ("only") is just a string
```

### dict
Stores **key → value** pairs. Python's main data structure: JSON, YAML, configs, API responses — everything turns into dictionaries. Inside it's a hash table, so a key lookup is instant (O(1)) regardless of size. Since Python 3.7 a dictionary **preserves the insertion order** of keys.

```python
cfg = {"env": "prod", "replicas": 3}

cfg["region"] = "us-east-2"   # add or overwrite a key
cfg["env"]                    # → 'prod'
cfg["timeout"]                # → KeyError — there's no such key, the program crashes
cfg.get("timeout")            # → None — safe access without an error
cfg.get("timeout", 30)        # → 30 — with a default value
"env" in cfg                  # → True — checks a key (not a value!)
del cfg["region"]             # delete a key (KeyError if absent)
cfg.pop("region", None)       # delete without an error, return the value or None

# iteration
for key in cfg:                     # keys are iterated by default
    print(key)
for key, value in cfg.items():      # keys and values at once — the most common variant
    print(f"{key} = {value}")
list(cfg.keys()), list(cfg.values())

# merging (3.9+): the right-hand dictionary overwrites matching keys
defaults = {"replicas": 1, "timeout": 30}
override = {"replicas": 3}
final = defaults | override         # → {'replicas': 3, 'timeout': 30}
# the old way: {**defaults, **override}
```

**setdefault** — "get the value, and if the key is missing, first create it with this value". Handy for grouping:
```python
by_ns = {}
for name, ns in [("api", "prod"), ("web", "prod"), ("db", "data")]:
    by_ns.setdefault(ns, []).append(name)
# → {'prod': ['api', 'web'], 'data': ['db']}
```

**Nested structures** — this is what parsed JSON from `kubectl get pod -o json` looks like:
```python
pod = {
    "metadata": {"name": "api-7d9f", "labels": {"app": "api"}},
    "status": {"containerStatuses": [{"name": "app", "restartCount": 3}]},
}
pod["metadata"]["labels"]["app"]                  # → 'api'
pod["status"]["containerStatuses"][0]["restartCount"]   # → 3
# safe access when a key may be missing:
pod.get("spec", {}).get("nodeName", "not scheduled")   # → 'not scheduled'
```

### set
An unordered collection of **unique** values. Why:
- removing duplicates: `set(["a", "b", "a"])` → `{'a', 'b'}`;
- a fast "is the element there" check (O(1), like in a dict);
- set operations — "what's there but not here".

```python
running = {"api", "web", "worker"}
expected = {"api", "web", "db"}

expected - running    # → {'db'}             — what's missing (expected but not running)
running - expected    # → {'worker'}         — what's extra
running & expected    # → {'api', 'web'}     — the intersection: present in both
running | expected    # → all four            — the union
running ^ expected    # → {'db', 'worker'}   — present in only one of the two

empty = set()         # an empty set. {} is an empty DICTIONARY, not a set!
running.add("cron"); running.discard("nope")   # discard doesn't fail if the element is absent
```
A real-life example: comparing the list of servers in an Ansible inventory with the EC2 instances actually running is exactly a set difference.

### The collections module — specialised collections
```python
from collections import Counter, defaultdict, deque

# Counter — a counting dictionary: counts how many times each element occurred
codes = ["200", "500", "200", "404", "200"]
c = Counter(codes)            # → Counter({'200': 3, '500': 1, '404': 1})
c.most_common(2)              # → [('200', 3), ('500', 1)] — the top 2
c["502"]                      # → 0 — a missing key gives 0, not KeyError

# defaultdict — a dictionary that creates the value for a new key itself
groups = defaultdict(list)    # automatically creates [] for a new key
groups["prod"].append("api")  # no need to check whether the key exists
# → defaultdict(<class 'list'>, {'prod': ['api']})

# deque — a double-ended queue; maxlen — keeps only the last N elements
last_lines = deque(maxlen=3)
for line in ["a", "b", "c", "d"]:
    last_lines.append(line)
# → deque(['b', 'c', 'd']) — the equivalent of tail -n 3
```

### Which collection to choose
| Task | Collection |
|---|---|
| An ordered collection that will change | `list` |
| A fixed "record", a dict key, returning several values | `tuple` |
| Lookup by key, a config, JSON | `dict` |
| Uniqueness, a fast `in`, comparing sets | `set` |
| Counting frequencies | `Counter` |
| Grouping | `defaultdict(list)` |
| The last N elements, a queue | `deque` |

### 🧪 Mini task

**Task.** You have the expected services (`api`, `web`, `db`), the ones actually running (`api`, `web`, `worker` and `web` again) and a list of HTTP statuses. Find which services are missing, which are extra, which run twice, the top 2 statuses, and group the statuses into `2xx`/`4xx`/`5xx` classes.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
from collections import Counter, defaultdict

expected = {"api", "web", "db"}
running = ["api", "web", "worker", "web"]          # web is running twice
statuses = ["200", "500", "200", "404", "200", "500"]

print("missing:", sorted(expected - set(running)))
print("extra:  ", sorted(set(running) - expected))
print("dupes:  ", [n for n, c in Counter(running).items() if c > 1])
print("top:    ", Counter(statuses).most_common(2))

by_class = defaultdict(list)
for s in statuses:
    by_class[s[0] + "xx"].append(s)
print("groups: ", dict(by_class))
EOF
```

Expected output:

```text
missing: ['db']
extra:   ['worker']
dupes:   ['web']
top:     [('200', 3), ('500', 2)]
groups:  {'2xx': ['200', '200', '200'], '5xx': ['500', '500'], '4xx': ['404']}
```

Breakdown: set difference answers "what is missing" and "what is extra" in one operation. `sorted()` is only there for stable output — a `set` has no order. `Counter` finds the duplicates: `web` has a count of 2. `most_common(2)` returns "value — count" pairs. `defaultdict(list)` creates an empty list for a new key, so grouping needs no `if key in` checks.

</details>

## Conditions and loops

### if / elif / else
```python
code = 503
if code < 300:
    status = "ok"
elif code < 500:          # elif = else if; checked only if the previous conditions are false
    status = "client error"
else:
    status = "server error"

# the ternary operator — an if in one expression
status = "ok" if code < 400 else "fail"

# chained comparisons — read like in maths
if 200 <= code < 300:
    print("2xx")

# a membership check instead of a chain of or
if env in ("prod", "production"):     # instead of env == "prod" or env == "production"
    print("careful, it's prod")
```

### for — iterating over a collection
In Python `for` iterates over **elements**, not indexes (like `foreach` in C#):
```python
for pod in ["api", "web"]:
    print(pod)

# if you also need the index — enumerate (not range(len(...)))
for i, pod in enumerate(["api", "web"], start=1):
    print(f"{i}. {pod}")     # → 1. api / 2. web

# iterating over two lists in parallel — zip
names = ["api", "web"]
ips = ["10.0.0.1", "10.0.0.2"]
for name, ip in zip(names, ips):
    print(name, ip)
# zip stops at the shortest list; strict=True (3.10+) fails if the lengths differ

# numbers — range(start, end, step), the end is NOT included
for i in range(5):          # 0, 1, 2, 3, 4
    print(i)
for i in range(10, 0, -2):  # 10, 8, 6, 4, 2
    print(i)
```

### while, break, continue and a loop's else
```python
attempts = 0
while attempts < 5:
    attempts += 1               # Python has no ++ ; you write += 1
    if attempts == 2:
        continue                # go to the next iteration, skipping the code below
    if check_health():
        print("the service is up")
        break                   # exit the loop
else:
    # a loop's else runs if the loop finished BY ITSELF, without a break
    raise RuntimeError("the service didn't come up in 5 attempts")
```
A loop's `else` is confusing, but it's useful precisely for searching and retries: "if we went through everything and didn't find it (there was no break) — do such-and-such".

### match — pattern matching (3.10+)
Similar to `switch`, but it can destructure data:
```python
def handle(event: dict) -> str:
    match event:
        case {"type": "deploy", "env": "prod"}:
            return "a prod deployment — needs approval"
        case {"type": "deploy", "env": env}:      # env is "pulled out" of the dictionary into a variable
            return f"deploying to {env}"
        case {"type": "alert", "severity": "critical" | "high"}:   # | means "or"
            return "page the on-call engineer"
        case _:                                   # _ — everything else (default)
            return "ignore"

handle({"type": "deploy", "env": "stage"})   # → 'deploying to stage'
```

### The "walrus" operator `:=`
It assigns a value and returns it right away. Needed so you don't compute the same thing twice:
```python
import re
if (m := re.search(r"took (\d+)ms", line)) is not None:
    print(int(m.group(1)))
# without the walrus you'd have to write: m = re.search(...); if m is not None: ...
```

### 🧪 Mini task

**Task.** Write a wait loop: up to 5 attempts to check a service's health, exit on success, and if all attempts fail, report it through the loop's `else`. Check two scenarios: the service comes up on the 3rd attempt, and it never comes up. Then classify the codes 204, 301, 404, 503 with `match`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
def make_check(ok_on):
    calls = {"n": 0}
    def check():
        calls["n"] += 1
        return calls["n"] == ok_on
    return check

for ok_on in (3, 99):
    check = make_check(ok_on)
    for attempt in range(1, 6):
        if check():
            print(f"up on attempt {attempt}")
            break
    else:
        print("still down after 5 attempts")

for code in (204, 301, 404, 503):
    match code:
        case 200 | 201 | 204:
            kind = "ok"
        case c if 300 <= c < 400:
            kind = "redirect"
        case c if c >= 500:
            kind = "server error"
        case _:
            kind = "client error"
    print(code, kind)
EOF
```

Expected output:

```text
up on attempt 3
still down after 5 attempts
204 ok
301 redirect
404 client error
503 server error
```

Breakdown: in the first scenario `break` fires and the `else` is skipped. In the second the loop finishes on its own, without `break`, so the `else` runs. That is exactly the "retry N times, otherwise fail" pattern from the section. In `match` the order of `case`s matters: the first matching one wins, and `case _` catches everything else. The `if` guards after a pattern let you check ranges.

</details>

## Comprehensions — building collections in one line

A comprehension is a compact form of a loop that builds a new collection. It's idiomatic Python: such code is usually shorter, easier to read and faster than a `for` with `append`.

```python
# an ordinary loop:
squares = []
for x in range(10):
    if x % 2 == 0:
        squares.append(x * x)

# the same with a list comprehension:
squares = [x * x for x in range(10) if x % 2 == 0]   # → [0, 4, 16, 36, 64]
# reads as: "x*x for each x in range(10), if x is even"
```

Kinds:
```python
pods = [
    {"name": "api", "ns": "prod", "restarts": 0},
    {"name": "web", "ns": "prod", "restarts": 7},
    {"name": "etl", "ns": "data", "restarts": 2},
]
names = [p["name"] for p in pods]                       # list  → ['api', 'web', 'etl']
by_name = {p["name"]: p["restarts"] for p in pods}      # dict  → {'api': 0, 'web': 7, 'etl': 2}
namespaces = {p["ns"] for p in pods}                    # set   → {'prod', 'data'}
broken = [p["name"] for p in pods if p["restarts"] > 5] # with a filter → ['web']
labels = ["ok" if p["restarts"] == 0 else "check" for p in pods]   # the if-else goes BEFORE the for
```
When **not** to use it: if there's complex logic inside, several conditions, or side effects (writing to a file, calling an API). A comprehension should build a collection, not "perform actions". If the line isn't readable at first glance — write an ordinary loop.

### 🧪 Mini task

**Task.** From the list of pods build, one line per item: a "name → restarts" dict for `prod` only; a sorted list of namespaces without repeats; a list of "noisy" pods (more than 5 restarts); labels like `api:ok` / `web:check`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
pods = [
    {"name": "api", "ns": "prod", "restarts": 0},
    {"name": "web", "ns": "prod", "restarts": 7},
    {"name": "etl", "ns": "data", "restarts": 2},
    {"name": "cron", "ns": "data", "restarts": 9},
]
prod = {p["name"]: p["restarts"] for p in pods if p["ns"] == "prod"}
namespaces = sorted({p["ns"] for p in pods})
noisy = [p["name"] for p in pods if p["restarts"] > 5]
labels = [f'{p["name"]}:{"ok" if p["restarts"] == 0 else "check"}' for p in pods]
print(prod)
print(namespaces)
print(noisy)
print(labels)
EOF
```

Expected output:

```text
{'api': 0, 'web': 7}
['data', 'prod']
['web', 'cron']
['api:ok', 'web:check', 'etl:check', 'cron:check']
```

Breakdown: an `if` filter at the end of a comprehension drops elements, while `if … else` *before* `for` picks a value for every element. These are two different syntaxes and people mix them up at interviews. The set comprehension removed the repeated `prod`/`data`, and `sorted` made the output stable.

</details>

## Generators — lazy sequences

**The problem.** You need to process a 20 GB log. `open(...).readlines()` reads the whole file into a list and eats 20+ GB of memory.

**The solution — a generator.** It hands out values **one at a time, on demand**, and only one value is in memory at any moment.

A generator expression is like a list comprehension, but in parentheses:
```python
total = sum(len(line) for line in open("huge.log"))   # memory for one line, not for the whole file
nums = (x * x for x in range(10**9))                  # created instantly — nothing has been computed yet
next(nums)   # → 0 — computes the next value only on demand
```

A generator function is a function with `yield` instead of `return`. Each `yield` hands out a value and *pauses* the function until the next request:
```python
def read_errors(path):
    """Yields the lines with ERROR one by one without holding the file in memory."""
    with open(path) as f:
        for line in f:                    # the file is also read line by line — lazily
            if "ERROR" in line:
                yield line.rstrip()       # hand out the line and "sleep" until the next request

def only_db(lines):
    for line in lines:
        if "database" in line:
            yield line

# generators can be chained into a pipeline, like pipes in bash: cat | grep | grep
for line in only_db(read_errors("/var/log/app.log")):
    print(line)
```
Important: a generator can be iterated **once**. After it has been consumed it's empty: a second `for` over it prints nothing. If you need another pass — turn it into a list with `list(gen)` (if it fits in memory) or create the generator again.

A DevOps example — API pagination as a generator: the calling code simply iterates over objects without thinking about pages:
```python
def all_projects(session, url):
    page = 1
    while True:
        r = session.get(url, params={"page": page, "per_page": 100}, timeout=10)
        r.raise_for_status()
        items = r.json()
        if not items:
            return            # return in a generator means the end of the sequence
        yield from items      # hand out each element of the list in turn
        page += 1
```

### 🧪 Mini task

**Task.** Generate a 100,000-line log (every 10th line is `ERROR`, every 20th belongs to `database`). Build a pipeline of two generators "ERROR only" → "database only", take the first line with `next()`, count the rest, try to iterate the generator a second time, and compare the size of a list of lines with the size of a generator.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import sys

with open("/tmp/app.log", "w") as f:              # 100,000 lines, every 10th is ERROR
    for i in range(100_000):
        level = "ERROR" if i % 10 == 0 else "INFO"
        svc = "database" if i % 20 == 0 else "api"
        f.write(f"{level} {svc} request {i}\n")

def read_errors(path):
    with open(path) as f:
        for line in f:
            if line.startswith("ERROR"):
                yield line.rstrip()

def only(word, lines):
    for line in lines:
        if word in line:
            yield line

pipeline = only("database", read_errors("/tmp/app.log"))
print(next(pipeline))                             # first line — nothing else has been read
print("db errors:", 1 + sum(1 for _ in pipeline))
print("second pass:", sum(1 for _ in pipeline))   # the generator is already exhausted

lines = open("/tmp/app.log").readlines()
gen = (l for l in open("/tmp/app.log"))
print("list KB:", sys.getsizeof(lines) // 1024, "| generator bytes:", sys.getsizeof(gen))
EOF
```

Expected output:

```text
ERROR database request 0
db errors: 5000
second pass: 0
list KB: 782 | generator bytes: 216
```

Breakdown: `next()` computed exactly one value — the file was read only up to the first matching line. The count is 5000 because every 20th line is both `ERROR` and `database`. The second pass gives 0: a generator is single-use. The list keeps all 100,000 lines in memory (about 780 KB of references alone, not counting the strings), while the generator takes about 200 bytes regardless of the file size. The exact numbers depend on the Python version.

</details>

## Functions

### Declaration, arguments, return
```python
def scale(service: str, replicas: int = 1) -> str:
    """Scales a service. This is a docstring — the function's description, visible in help(scale)."""
    return f"{service} -> {replicas}"

scale("api")                    # → 'api -> 1'   — replicas takes the default
scale("api", 3)                 # → 'api -> 3'   — positional arguments
scale(replicas=3, service="api")  # keyword arguments — the order doesn't matter, more readable
```
`: str` and `-> str` are type hints. Python **doesn't check** them at runtime; they're for the reader, the IDE and mypy (a static analyser: it checks types from the hints without running the code, usually in CI).

### *args and **kwargs — any number of arguments
```python
def log(level, *messages, **fields):
    # *messages collects all the "extra" positional arguments into a TUPLE
    # **fields collects all the "extra" keyword arguments into a DICTIONARY
    print(level, messages, fields)

log("INFO", "deploy", "started", service="api", env="prod")
# → INFO ('deploy', 'started') {'service': 'api', 'env': 'prod'}
```
The names `args`/`kwargs` are a convention; only `*` and `**` matter. The main use is wrapper functions and decorators that accept "anything" and pass it on: `func(*args, **kwargs)`.

The reverse operation is **unpacking** at the call site:
```python
params = {"service": "web", "replicas": 2}
scale(**params)              # the same as scale(service="web", replicas=2)
args = ["web", 2]
scale(*args)                 # the same as scale("web", 2)
```

**Keyword-only arguments.** Everything after `*` in the declaration can be passed only by name:
```python
def delete_namespace(name, *, force=False):
    args = ["kubectl", "delete", "namespace", name]
    if force:
        args += ["--grace-period=0", "--force"]
    subprocess.run(args, check=True)
delete_namespace("dev", True)        # → TypeError
delete_namespace("dev", force=True)  # ok — from the call you can immediately see what the True is
```
Why: the call `delete_namespace("dev", True)` is unreadable — what does `True` mean? For flags this is good practice.

### The trap: a mutable default value
The most frequent interview question about functions.
```python
def add_tag(tag, tags=[]):
    tags.append(tag)
    return tags

add_tag("a")   # → ['a']
add_tag("b")   # → ['a', 'b']   (!) expected ['b']
```
**Why:** the default value is evaluated **once**, when the function is defined (when Python executes the `def` line), not on every call. All calls without the argument get *the same* list and accumulate data in it.

**The right way:** use `None` as a "not passed" marker and create a new object inside:
```python
def add_tag(tag, tags=None):
    if tags is None:
        tags = []          # a new list on EVERY call
    tags.append(tag)
    return tags
```
The same applies to `{}` and `set()` as defaults.

### Scope: LEGB
When Python encounters a name, it looks for it in this order:
1. **L**ocal — inside the current function;
2. **E**nclosing — in the outer function (if the function is nested);
3. **G**lobal — at the module (file) level;
4. **B**uilt-in — the built-in names (`len`, `print`, `list`).

```python
counter = 0
def increment():
    counter += 1     # UnboundLocalError!
```
Why the error: an assignment inside a function makes `counter` a *local* variable for the whole function, and it can't be read before it's assigned. To change the global one, you need `global counter`. But it's better not to do that: global state complicates the code and the tests. Pass values as arguments and return them via `return`.

Another trap: don't name variables after built-in functions (`list = [1, 2]`, `id = 5`, `type = "x"`). You'll "shadow" the built-in function, and `list("abc")` further down the file will break.

### lambda — a short anonymous function
```python
square = lambda x: x * x          # the same as def square(x): return x * x
sorted(pods, key=lambda p: p["restarts"])
```
The limitation: only one expression, no statements. It's used where a small function is needed "on the spot" — in `key=` for sorting, `max`, `min`, `filter`. If the logic is more complex — write an ordinary `def`.

### Closures
A nested function "remembers" the outer function's variables even after the outer function has finished:
```python
def make_prefixer(prefix):
    def add_prefix(name):
        return f"{prefix}-{name}"    # prefix is taken from the outer function
    return add_prefix                # return the function itself without calling it

prod = make_prefixer("prod")
prod("api")    # → 'prod-api'
```
Decorators are built on closures.

### 🧪 Mini task

**Task.** Reproduce the mutable default argument trap and fix it with `None`. Then write `scale(service, *, replicas=1, dry_run=True)` with keyword-only arguments and show that the call `scale("api", 3)` is rejected.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
def add_tag_bad(tag, tags=[]):
    tags.append(tag)
    return tags

def add_tag(tag, tags=None):
    if tags is None:
        tags = []
    tags.append(tag)
    return tags

print(add_tag_bad("a"), add_tag_bad("b"))
print(add_tag("a"), add_tag("b"))

def scale(service, *, replicas=1, dry_run=True):
    return f"{'[dry-run] ' if dry_run else ''}{service} -> {replicas}"

print(scale("api", replicas=3))
try:
    scale("api", 3)
except TypeError as e:
    print("TypeError:", e)
EOF
```

Expected output:

```text
['a', 'b'] ['a', 'b']
['a'] ['b']
[dry-run] api -> 3
TypeError: scale() takes 1 positional argument but 2 were given
```

Breakdown: in the first line both calls returned *the same* list, and it accumulated `['a', 'b']` — the default list is created once, at `def` time. Note that it is printed after the second call, so both values look identical. In the second line each call created its own list. The `*` in the signature forbids passing `replicas` positionally, so the unclear call `scale("api", 3)` fails immediately instead of doing something unexpected.

</details>

## Decorators

**The problem.** A dozen functions that call an API need to retry on network errors. Copying the retry loop into each one is bad.

**The solution.** A decorator is a function that takes a function and returns a *new* function with added behaviour. The `@decorator` syntax above a `def` is shorthand for `func = decorator(func)`.

The simplest decorator — measuring execution time:
```python
import functools
import time

def timed(func):
    @functools.wraps(func)                # keep the original function's name and docstring
    def wrapper(*args, **kwargs):         # accepts any arguments...
        start = time.perf_counter()
        result = func(*args, **kwargs)    # ...and passes them to the original function
        print(f"{func.__name__} took {time.perf_counter() - start:.2f}s")
        return result
    return wrapper

@timed
def backup():
    time.sleep(1)

backup()   # → backup took 1.00s
```
Without `functools.wraps` the wrapped function's name becomes `wrapper`, which confuses logs and debugging.

A decorator with parameters needs one more level of nesting (a function that *returns* a decorator):
```python
def retry(times=3, delay=1.0, exceptions=(Exception,)):
    def decorator(func):
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            for attempt in range(1, times + 1):
                try:
                    return func(*args, **kwargs)
                except exceptions as e:
                    if attempt == times:
                        raise                          # out of attempts — re-raise the error
                    wait = delay * 2 ** (attempt - 1)  # exponential backoff: 1, 2, 4, 8 s
                    print(f"{func.__name__}: attempt {attempt} failed ({e}), waiting {wait}s")
                    time.sleep(wait)
        return wrapper
    return decorator

@retry(times=5, exceptions=(ConnectionError, TimeoutError))
def fetch_status():
    return requests.get("https://api.example.com/status", timeout=5).json()
```
Why exponential backoff: if the service is overloaded, frequent retries from all the clients will finish it off. A growing pause gives it time to recover. In production random spread (jitter) is also added so that clients don't retry in sync.

Built-in decorators you should know:
- `@functools.cache` / `@functools.lru_cache(maxsize=128)` — remembers the result for the same arguments (memoisation);
- `@property` — a method that's accessed like an attribute (`svc.is_ha` without parentheses);
- `@staticmethod`, `@classmethod` — see the section on classes;
- `@dataclass` — generates a class's methods.

### 🧪 Mini task

**Task.** Write a `@retry(times=3, exceptions=(ConnectionError,))` decorator without delays and apply it to a function that fails twice and answers on the third try. Check that the function keeps its name and docstring after wrapping.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import functools

def retry(times=3, exceptions=(Exception,)):
    def decorator(func):
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            for attempt in range(1, times + 1):
                try:
                    return func(*args, **kwargs)
                except exceptions as e:
                    print(f"{func.__name__}: attempt {attempt} failed: {e}")
                    if attempt == times:
                        raise
        return wrapper
    return decorator

calls = 0

@retry(times=3, exceptions=(ConnectionError,))
def fetch_status():
    """Fails twice, answers on the third try."""
    global calls
    calls += 1
    if calls < 3:
        raise ConnectionError("connection reset")
    return {"status": "ok"}

print(fetch_status())
print(fetch_status.__name__, "|", fetch_status.__doc__)
EOF
```

Expected output:

```text
fetch_status: attempt 1 failed: connection reset
fetch_status: attempt 2 failed: connection reset
{'status': 'ok'}
fetch_status | Fails twice, answers on the third try.
```

Breakdown: two failed attempts are logged, the third returned a result, and the calling code never even noticed the failures. If the function had failed all three times, `raise` would have re-raised the last error. The last line is thanks to `@functools.wraps`: without it you would see `wrapper | None`. Try removing `wraps` and running again.

</details>

## Exceptions

**Why.** An error (no file, the network is unavailable, invalid JSON) interrupts execution and "flies" up the call stack until someone catches it. If nobody does, the program crashes with a traceback (which function called which and where it failed).

```python
import json

raw = '{"replicas": 3'   # broken JSON — no closing brace
try:
    cfg = json.loads(raw)
except json.JSONDecodeError as e:          # catch a SPECIFIC error type
    print(f"the config is broken: {e}")
    cfg = {"replicas": 1}                  # and decide what to do: a default value
else:
    print("the config was read")           # runs only if there was NO exception
finally:
    print("always runs")                   # close resources, release a lock
```

Catching several types and re-raising:
```python
try:
    value = cfg["timeout"] / int(cfg["retries"])
except (KeyError, ValueError) as e:
    print(f"bad config: {e!r}")
except ZeroDivisionError:
    value = 0
except Exception:
    log.exception("unexpected error")      # logs the message AND the traceback
    raise                                  # re-raise the same error upwards — don't "swallow" it
```

Your own exceptions — for clear domain errors:
```python
class DeployError(Exception):
    """A deployment error."""

def deploy(service):
    try:
        run_helm(service)
    except subprocess.CalledProcessError as e:
        raise DeployError(f"helm failed for {service}") from e
        # from e keeps the original cause: the traceback will show both errors
```

**The rules:**
- Catch **specific** exceptions. A bare `except:` catches absolutely everything, including `KeyboardInterrupt` (Ctrl+C) and `SystemExit`, and the script stops shutting down normally.
- Don't silently "swallow" errors (`except Exception: pass`). It's the worst option: something broke, and you'll never know.
- **EAFP versus LBYL.** In Python the convention is "try first, then handle the error" (EAFP) rather than "check everything in advance" (LBYL):
  ```python
  # LBYL: check in advance. The file may be deleted between the check and the open (a race condition)
  if os.path.exists(path):
      data = open(path).read()
  # EAFP: just try
  try:
      data = open(path).read()
  except FileNotFoundError:
      data = ""
  ```

The main built-in exceptions: `ValueError` (a wrong value: `int("abc")`), `TypeError` (a wrong type), `KeyError` (no key in a dict), `IndexError` (no index in a list), `FileNotFoundError`, `PermissionError`, `TimeoutError`, `ConnectionError`, `AttributeError` (the object has no such attribute, often when a variable is unexpectedly `None`).

### 🧪 Mini task

**Task.** Write `load(raw)` that gets `replicas` from a JSON string and turns any problem (broken JSON, a missing key) into one clear `ConfigError`, keeping the original cause with `from`. Run it on three strings: a valid one, one with broken JSON and one without the key.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import json

class ConfigError(Exception):
    """A clear error instead of KeyError/JSONDecodeError."""

def load(raw):
    try:
        cfg = json.loads(raw)
        return int(cfg["replicas"])
    except json.JSONDecodeError as e:
        raise ConfigError(f"broken JSON at pos {e.pos}") from e
    except KeyError as e:
        raise ConfigError(f"missing key {e}") from e

for raw in ['{"replicas": "3"}', '{"replicas": 3', '{"image": "api"}']:
    try:
        print("ok:", load(raw))
    except ConfigError as e:
        print(f"ConfigError: {e} (cause: {type(e.__cause__).__name__})")
EOF
```

Expected output:

```text
ok: 3
ConfigError: broken JSON at pos 14 (cause: JSONDecodeError)
ConfigError: missing key 'replicas' (cause: KeyError)
```

Breakdown: the calling code catches only `ConfigError` and doesn't need to know about `json` or `KeyError`. The original error is not lost: `from e` stores it in `__cause__`, and the traceback shows both. `e.pos` is the position in the string where the parser stumbled — useful for the user-facing message. The first line also shows that `int("3")` turned the string from JSON into a number.

</details>

## Context managers: with

**The problem.** A file, a network connection or a lock must be closed or released even if an error happened mid-way. If you forget — descriptor leaks (`Too many open files`), stuck locks.

**The solution — `with`.** It guarantees the resource is released on leaving the block, however you leave it: normally, via `return` or via an exception.
```python
with open("deploy.log", "a") as f:     # open the file for appending
    f.write("deploy started\n")
# here the file is already closed — even if write failed with an error

# several resources at once
with open("in.txt") as src, open("out.txt", "w") as dst:
    dst.write(src.read().upper())
```
Without `with` you'd have to write `try/finally` with `f.close()` by hand.

The easiest way to make your own context manager is a generator plus the `@contextmanager` decorator. The code before `yield` is the "entry", the code after it is the "exit":
```python
from contextlib import contextmanager
import time

@contextmanager
def timer(name):
    start = time.perf_counter()
    try:
        yield                          # the body of the with block runs here
    finally:
        print(f"{name}: {time.perf_counter() - start:.2f}s")

with timer("backup"):
    time.sleep(2)                      # → backup: 2.00s
```
Other useful ones: `tempfile.TemporaryDirectory()` (a temporary folder that deletes itself), `contextlib.suppress(FileNotFoundError)` (ignore a specific error), `threading.Lock()`.

### 🧪 Mini task

**Task.** Write a `cd(path)` context manager — the equivalent of `pushd`/`popd`: inside `with` the working directory is `path`, and after exit it is the previous one, even if the block raised. Check it on a temporary directory that removes itself.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import os, tempfile
from contextlib import contextmanager

@contextmanager
def cd(path):
    old = os.getcwd()
    os.chdir(path)
    try:
        yield path
    finally:
        os.chdir(old)          # go back even if the block raised

start = os.getcwd()
with tempfile.TemporaryDirectory() as tmp:
    with cd(tmp):
        print("inside tmp:", os.getcwd() == os.path.realpath(tmp))
    try:
        with cd(tmp):
            raise RuntimeError("boom")
    except RuntimeError as e:
        print("caught:", e)
    print("back home:", os.getcwd() == start)
print("tmp removed:", not os.path.exists(tmp))
EOF
```

Expected output:

```text
inside tmp: True
caught: boom
back home: True
tmp removed: True
```

Breakdown: the `try/finally` around `yield` guarantees the return to the original directory: the `boom` exception went through the manager, yet `back home` is still `True`. `realpath` is needed because on macOS `/tmp` is a symlink to `/private/tmp`, otherwise the path strings would not match. `TemporaryDirectory` is a context manager too: once its `with` exits, the directory is gone.

</details>

## Classes and objects

**Why classes.** To bring together data and the functions that work with it. For example, a "service" is a name, a number of replicas, ports, and the operations "scale" and "check HA". Automation scripts need classes less often than applications do. Dictionaries and functions are often enough, but you need to be able to read and write classes.

### An ordinary class
```python
class Service:
    default_port = 8080                  # a CLASS attribute — shared by all instances

    def __init__(self, name, replicas=1):
        # __init__ is the initialiser: called when the object is created with Service(...)
        self.name = name                 # INSTANCE attributes — each object has its own
        self.replicas = replicas

    def scale(self, n):                  # a method — a function inside a class
        if n < 0:
            raise ValueError("replicas can't be negative")
        self.replicas = n

    def __repr__(self):                  # how the object is shown in print and when debugging
        return f"Service({self.name!r}, replicas={self.replicas})"

api = Service("api", 3)                  # create an instance
api.scale(5)
print(api)                               # → Service('api', replicas=5)
```
**What `self` is.** A reference to the specific object the method was called on. Python turns `api.scale(5)` into `Service.scale(api, 5)`. That's why `self` is the first parameter of every method and is written explicitly (in C# it's the implicit `this`).

**Magic (dunder) methods** — methods with double underscores through which an object plugs into the language's syntax: `__init__` (creation), `__repr__`/`__str__` (printing), `__eq__` (comparison with `==`), `__len__` (`len(obj)`), `__iter__` (`for x in obj`), `__enter__`/`__exit__` (`with`).

### Inheritance
```python
class StatefulService(Service):          # inherit everything from Service
    def __init__(self, name, replicas=1, storage="10Gi"):
        super().__init__(name, replicas) # call the parent's initialiser
        self.storage = storage

    def scale(self, n):                  # override the parent's method
        if n < self.replicas:
            raise ValueError("a stateful service can't be scaled down without migrating data")
        super().scale(n)                 # and call the parent's implementation

db = StatefulService("postgres", 3)
isinstance(db, Service)                  # → True — the subclass "is a" parent
```

### @property, @classmethod, @staticmethod
```python
class Service:
    def __init__(self, name, replicas):
        self.name = name
        self.replicas = replicas

    @property
    def is_ha(self):                     # accessed like an attribute: svc.is_ha (without parentheses)
        return self.replicas >= 2

    @classmethod
    def from_dict(cls, d):               # receives the CLASS (cls), not an instance
        return cls(d["name"], d.get("replicas", 1))   # an alternative constructor

    @staticmethod
    def valid_name(name):                # receives neither the class nor an instance
        return name.isidentifier()       # just a function that logically belongs to the class

svc = Service.from_dict({"name": "api", "replicas": 3})
svc.is_ha                                # → True
```

### dataclass — a class for holding data without boilerplate
For classes that mainly hold data, writing `__init__`, `__repr__` and `__eq__` by hand is tedious. `@dataclass` generates them from the field annotations:
```python
from dataclasses import dataclass, field

@dataclass
class Service:
    name: str
    replicas: int = 1
    ports: list[int] = field(default_factory=list)   # NOT ports: list = [] — the same mutable default trap

    @property
    def is_ha(self) -> bool:
        return self.replicas >= 2

a = Service("api", 3)
b = Service("api", 3)
print(a)          # → Service(name='api', replicas=3, ports=[])   — __repr__ is generated
a == b            # → True   — __eq__ compares the fields (an ordinary class would give False)
```
`@dataclass(frozen=True)` makes the object immutable (and hashable — it can be put into a set).

### "Privacy" conventions
Python has no real `private`. There are conventions:
- `_name` — "internal, don't touch from outside". Technically accessible.
- `__name` — the name is mangled into `_ClassName__name` so that subclasses don't overwrite it by accident. Rarely used.

### 🧪 Mini task

**Task.** Describe a service as a `dataclass` with `name`, `replicas=1`, `ports` (an empty list by default), an `is_ha` property and a `from_dict` alternative constructor. Build services from a list of dicts (some lack `replicas` and `ports`), print them by replicas in descending order with an HA mark, and compare two identical objects with `==`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
from dataclasses import dataclass, field

@dataclass
class Service:
    name: str
    replicas: int = 1
    ports: list[int] = field(default_factory=list)

    @property
    def is_ha(self):
        return self.replicas >= 2

    @classmethod
    def from_dict(cls, d):
        return cls(d["name"], d.get("replicas", 1), d.get("ports", []))

raw = [{"name": "api", "replicas": 3, "ports": [8080]}, {"name": "cron"}, {"name": "web", "replicas": 2}]
services = [Service.from_dict(d) for d in raw]
for s in sorted(services, key=lambda s: s.replicas, reverse=True):
    print(s, "HA" if s.is_ha else "single")
print(Service("api", 3, [8080]) == services[0])
EOF
```

Expected output:

```text
Service(name='api', replicas=3, ports=[8080]) HA
Service(name='web', replicas=2, ports=[]) HA
Service(name='cron', replicas=1, ports=[]) single
True
```

Breakdown: `__repr__` and `__eq__` were generated by `@dataclass`, so the output is readable and `==` compares fields and gives `True`. A plain class would give `False`, because object identities would be compared. `from_dict` filled in the defaults for `cron`. `is_ha` is accessed without parentheses because it is a `@property`.

</details>

## Modules, packages and imports

A **module** is any `.py` file. A **package** is a folder of modules (usually with an `__init__.py` file).
```
mytool/
  pyproject.toml
  src/mytool/
    __init__.py
    cli.py          # the entry point
    k8s.py          # functions for working with the cluster
  tests/
    test_k8s.py
```
```python
import os                              # import the whole module: os.getenv(...)
from pathlib import Path               # import a specific name: Path(...)
from mytool.k8s import get_pods        # from your own package
import datetime as dt                  # an alias
```
Avoid `from module import *`: it's unclear which names appeared, and they may shadow yours.

### `if __name__ == "__main__":`
```python
# file: cleanup.py
def cleanup(days):
    print(f"cleaning up older than {days} days")

if __name__ == "__main__":
    cleanup(30)
```
When the file is run directly (`python cleanup.py`), Python sets the `__name__` variable to `"__main__"`, and the block runs. When the file is *imported* (`from cleanup import cleanup`), `__name__` equals `"cleanup"`, and the block doesn't run. This way one file can be both a script and a library: its functions can be imported into tests without running the main logic.

### 🧪 Mini task

**Task.** Create a `mytool` package with a `k8s.py` module that has a function and an `if __name__ == "__main__":` block. Run the module as a script (`python3 -m mytool.k8s`), then import the function from other code and compare what is printed in each case.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
mkdir mytool
touch mytool/__init__.py
cat > mytool/k8s.py <<'EOF'
print(f"k8s.py loaded, __name__ = {__name__}")

def noisy(pods, threshold=3):
    return [name for name, restarts in pods.items() if restarts > threshold]

if __name__ == "__main__":
    print("run as script:", noisy({"api": 0, "web": 7}))
EOF
echo '--- python3 -m mytool.k8s'
python3 -m mytool.k8s
echo '--- import'
python3 -c "from mytool.k8s import noisy; print('imported:', noisy({'etl': 9}))"
```

Expected output:

```text
--- python3 -m mytool.k8s
k8s.py loaded, __name__ = __main__
run as script: ['web']
--- import
k8s.py loaded, __name__ = mytool.k8s
imported: ['etl']
```

Breakdown: module-level code (the first `print`) runs in both cases — importing executes the file. The block under `if __name__ == "__main__"` ran only when started as a script, because on import `__name__` equals the module name `mytool.k8s`. That is why modules don't keep real work at the top level: importing them in tests would run it.

</details>

## Type hints
```python
from typing import Iterator, TypedDict

class Pod(TypedDict):
    name: str
    restarts: int

def noisy_pods(pods: list[Pod], threshold: int = 3) -> list[str]:
    return [p["name"] for p in pods if p["restarts"] > threshold]

def read_lines(path: str) -> Iterator[str]:   # a generator of lines
    with open(path) as f:
        yield from f

def find(pods: list[Pod], name: str) -> Pod | None:   # may return a Pod or None
    return next((p for p in pods if p["name"] == name), None)
```
**Why, if Python doesn't check them:** the IDE suggests methods and highlights errors; `mypy` in CI finds bugs before running (for example, you passed a string where a number is expected, or forgot to handle `None`); and it's documentation that doesn't go stale. In large scripts and teams it's the standard.

### 🧪 Mini task

**Task.** Write a file with two type-hinted functions and two hidden bugs: `.get()` may return `None`, and a string is passed in a dict instead of a number. Find both bugs with `mypy` *before* running, then run the file and see how the first one shows up at runtime.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > pods.py <<'EOF'
def find_restarts(pods: dict[str, int], name: str) -> int:
    return pods.get(name)          # .get() may return None

def total(pods: dict[str, int]) -> int:
    return sum(pods.values())

print(find_restarts({"api": 2}, "web") + 1)
print(total({"api": "2"}))
EOF
mypy pods.py
python3 pods.py 2>&1 | tail -1
```

Expected output:

```text
pods.py:2: error: Incompatible return value type (got "int | None", expected "int")  [return-value]
pods.py:8: error: Dict entry 0 has incompatible type "str": "str"; expected "str": "int"  [dict-item]
Found 2 errors in 1 file (checked 1 source file)
TypeError: unsupported operand type(s) for +: 'NoneType' and 'int'
```

Breakdown: `mypy` found both bugs statically and pointed at the lines, while Python itself doesn't check hints: the script started and failed only on the first bug, with a less clear `TypeError` about `NoneType`. The runtime would never have reached the second bug — summing a string with a number would fail later or elsewhere. Fix for the first: return `pods.get(name, 0)`, or change the type to `int | None` and handle `None`. Install `mypy` with `brew install mypy` or run it as `uvx mypy`.

</details>

## Working with files and paths
```python
from pathlib import Path

log_dir = Path("/var/log") / "app"           # the / operator joins paths (cross-platform)
log_dir.exists(), log_dir.is_dir()
for f in log_dir.glob("*.log"):              # files by mask; rglob — recursively
    print(f.name, f.suffix, f.stat().st_size)  # the name, extension, size in bytes

cfg = Path("config.yaml")
text = cfg.read_text(encoding="utf-8")       # read the whole file into a string
Path("out.txt").write_text("done\n")         # write (overwrite)
Path("data/raw").mkdir(parents=True, exist_ok=True)   # like mkdir -p
Path("old.log").unlink(missing_ok=True)      # delete without failing if the file doesn't exist

# reading a large file line by line — without loading it into memory
with open("/var/log/app.log", encoding="utf-8", errors="replace") as f:
    for line in f:                           # a file object is itself a generator of lines
        process(line)
```
`open` modes: `"r"` — reading (the default), `"w"` — writing with the file **overwritten**, `"a"` — appending to the end, `"rb"`/`"wb"` — binary mode. `errors="replace"` replaces "broken" bytes instead of failing with `UnicodeDecodeError` — useful for logs.

### 🧪 Mini task

**Task.** In a temporary folder create a `var/log/app` tree (like `mkdir -p`) with files of different sizes, print only `*.log` by size in descending order, write a report with names without the extension into a neighbouring file, and delete one log twice so that the second time there is no error.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import tempfile
from pathlib import Path

with tempfile.TemporaryDirectory() as tmp:
    logs = Path(tmp) / "var" / "log" / "app"
    logs.mkdir(parents=True, exist_ok=True)            # mkdir -p
    for name, size in [("api.log", 300), ("web.log", 1200), ("db.log", 50), ("notes.txt", 999)]:
        (logs / name).write_text("x" * size)

    files = sorted(logs.glob("*.log"), key=lambda p: p.stat().st_size, reverse=True)
    for p in files:
        print(f"{p.name:<8} {p.stat().st_size:>5} B  suffix={p.suffix}")

    report = logs.parent / "report.txt"
    report.write_text("\n".join(p.stem for p in files) + "\n")
    print("report:", report.read_text().split())
    (logs / "db.log").unlink(missing_ok=True)
    (logs / "db.log").unlink(missing_ok=True)          # second time — no error
    print("left:", sorted(p.name for p in logs.iterdir()))
EOF
```

Expected output:

```text
web.log   1200 B  suffix=.log
api.log    300 B  suffix=.log
db.log      50 B  suffix=.log
report: ['web', 'api', 'db']
left: ['api.log', 'notes.txt', 'web.log']
```

Breakdown: `glob("*.log")` skipped `notes.txt`. `stat().st_size` is the size in bytes, which is what we sort by. `stem` is the name without the extension, `suffix` is the extension with the dot. The `/` operator joins paths, and `logs.parent` goes one level up. `unlink(missing_ok=True)` silently did nothing the second time — the equivalent of `rm -f`.

</details>

## The standard library for DevOps

### os and sys: the environment and exiting the program
```python
import os, sys

region = os.environ.get("AWS_REGION", "us-east-2")   # an environment variable with a default
token = os.environ["GITLAB_TOKEN"]                   # a required one — KeyError if missing
if not os.getenv("GITLAB_TOKEN"):
    sys.exit("GITLAB_TOKEN is not set")             # a message to stderr and exit code 1
sys.exit(0)                                         # a successful exit
print("error", file=sys.stderr)                     # printing to stderr
sys.argv                                            # command-line arguments (a list of strings)
```
The exit code matters: CI, cron and systemd look exactly at it. 0 is success; everything else is an error.

### subprocess: running external commands
```python
import subprocess, json

result = subprocess.run(
    ["kubectl", "get", "pods", "-n", "prod", "-o", "json"],  # the command is a LIST of arguments
    capture_output=True,   # capture stdout and stderr (otherwise they go to the terminal)
    text=True,             # get str rather than bytes
    check=True,            # raise CalledProcessError on a non-zero exit code
    timeout=60,            # don't wait forever
)
pods = json.loads(result.stdout)
print(result.returncode)   # → 0
```
Why a list and not a string with `shell=True`:
```python
ns = input("namespace: ")     # the user types:  prod; rm -rf /
subprocess.run(f"kubectl get pods -n {ns}", shell=True)   # DANGEROUS: the shell will run rm -rf / as well
subprocess.run(["kubectl", "get", "pods", "-n", ns])      # safe: "prod; rm -rf /" is just one argument
```
With a list the shell isn't involved, and special characters (`;`, `|`, `$()`) aren't interpreted. As a bonus you don't have to think about quotes and spaces in arguments. `os.system` is an outdated way; don't use it.

Handling a command's error:
```python
try:
    subprocess.run(["helm", "upgrade", "api", "./chart"], check=True, capture_output=True, text=True)
except subprocess.CalledProcessError as e:
    print(f"helm exited with code {e.returncode}: {e.stderr}")
except subprocess.TimeoutExpired:
    print("helm didn't finish within the timeout")
```

### json and yaml
```python
import json
data = json.loads('{"a": 1}')            # a JSON string → dict
text = json.dumps(data, indent=2, ensure_ascii=False)   # dict → a pretty string (non-ASCII without \u escapes)
with open("out.json", "w") as f:
    json.dump(data, f, indent=2)         # straight into a file (load/dump — files, loads/dumps — strings)

import yaml                              # the pyyaml package: pip install pyyaml
with open("values.yaml") as f:
    values = yaml.safe_load(f)           # ALWAYS safe_load
docs = list(yaml.safe_load_all(open("manifests.yaml")))   # multi-document YAML (the --- separator)
print(yaml.safe_dump(values, sort_keys=False))
```
Why `safe_load` and not `load`: YAML can describe the creation of arbitrary Python objects (the `!!python/object` tag). `yaml.load` with an unsafe loader on an untrusted file means executing someone else's code.

### re — regular expressions
```python
import re
line = '10.0.0.5 - - [12/Oct/2026:10:00:00] "GET /api/health HTTP/1.1" 503 12'

m = re.search(r'^(\S+) .*"(\w+) (\S+) [^"]*" (\d{3})', line)
if m:                                    # search returns None if nothing was found — check it!
    ip, method, path, status = m.groups()
    # → ('10.0.0.5', 'GET', '/api/health', '503')

re.findall(r"\d+\.\d+\.\d+\.\d+", text)  # all IPs in the text → a list of strings
re.sub(r"password=\S+", "password=***", line)   # replacing by a pattern — masking secrets in logs

# named groups — more readable than numbers
m = re.search(r"(?P<ip>\S+) .* (?P<status>\d{3}) ", line)
m["ip"], m["status"]

# if a pattern is used in a loop — compile it in advance
STATUS = re.compile(r'" (\d{3}) ')
```
`re.search` searches anywhere in the string, `re.match` — only at the start of the string, `re.fullmatch` — the whole string.

### logging — instead of print
```python
import logging
logging.basicConfig(
    level=logging.INFO,                                       # DEBUG < INFO < WARNING < ERROR < CRITICAL
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)
log = logging.getLogger(__name__)

log.debug("debugging details")           # won't be printed at level=INFO
log.info("deployed %s to %s", "api", "prod")
log.warning("retry %d/%d", 2, 5)
try:
    1 / 0
except ZeroDivisionError:
    log.exception("something failed")    # the ERROR level + the full traceback
```
Why not `print`: levels (you can turn on DEBUG with a flag without editing the code), the time and source on every line, output to stderr (leaving stdout for useful data), easy to route to a file or JSON for Loki/ELK. Write `log.info("x=%s", x)`, not an f-string: the formatting happens only if the message is actually printed.

### argparse — command-line arguments
```python
import argparse

parser = argparse.ArgumentParser(description="Deletes EBS snapshots older than N days")
parser.add_argument("--days", type=int, default=30, help="age in days (30 by default)")
parser.add_argument("--region", default="us-east-2")
parser.add_argument("--dry-run", action="store_true", help="only show, don't delete anything")
parser.add_argument("tags", nargs="*", help="filter by tags")   # positional arguments
args = parser.parse_args()

print(args.days, args.dry_run, args.tags)   # a hyphen in the name turns into an underscore
```
`python cleanup.py --help` automatically prints the help, and wrong arguments produce a clear error.

### datetime — dates and times
```python
from datetime import datetime, timedelta, timezone

now = datetime.now(timezone.utc)                 # ALWAYS with a time zone
cutoff = now - timedelta(days=30)                # date arithmetic
now.isoformat()                                  # → '2026-10-12T10:00:00.123+00:00'
datetime.fromisoformat("2026-10-01T00:00:00+00:00")
now.strftime("%Y-%m-%d")                         # → '2026-10-12' — formatting
```
Why with `timezone.utc`: boto3 and most APIs return dates *with* a time zone ("aware"). Comparing an aware date with a naive one (without a zone) fails with `TypeError: can't compare offset-naive and offset-aware datetimes`. It's a common mistake in cleanup scripts.

### 🧪 Mini task

**Task.** Write a `logstat.py` CLI: it takes the path to an access.log and a `--fail-on-5xx` flag (argparse), parses lines with a named-group regex (re), prints a status counter as JSON (json) to stdout, logs 4xx to stderr while masking the password in the URL (logging), and with the flag exits with code 1 if there were 5xx (sys.exit). Run it without and with the flag and look at the exit code.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > access.log <<'EOF'
10.0.0.5 - - [12/Oct/2026:10:00:00] "GET /api/health HTTP/1.1" 200 12
10.0.0.7 - - [12/Oct/2026:10:00:01] "POST /api/deploy HTTP/1.1" 503 0
10.0.0.5 - - [12/Oct/2026:10:00:02] "GET /api/pods HTTP/1.1" 200 512
10.0.0.9 - - [12/Oct/2026:10:00:03] "GET /login?password=hunter2 HTTP/1.1" 401 0
EOF
cat > logstat.py <<'EOF'
import argparse, json, logging, re, sys
from collections import Counter

LINE = re.compile(r'^(?P<ip>\S+) .*"(?P<method>\w+) (?P<path>\S+) [^"]*" (?P<status>\d{3})')

def main():
    parser = argparse.ArgumentParser(description="Status codes from access.log")
    parser.add_argument("path")
    parser.add_argument("--fail-on-5xx", action="store_true")
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")

    statuses = Counter()
    with open(args.path) as f:
        for line in f:
            m = LINE.search(line)
            if not m:
                continue
            statuses[m["status"]] += 1
            if m["status"].startswith("4"):
                logging.warning("%s %s", m["status"], re.sub(r"password=\S+", "password=***", m["path"]))
    print(json.dumps(dict(sorted(statuses.items()))))
    if args.fail_on_5xx and any(s.startswith("5") for s in statuses):
        sys.exit("5xx found")

if __name__ == "__main__":
    main()
EOF
python3 logstat.py access.log; echo "exit=$?"
python3 logstat.py access.log --fail-on-5xx 2>/dev/null; echo "exit=$?"
```

Expected output:

```text
WARNING 401 /login?password=***
{"200": 2, "401": 1, "503": 1}
exit=0
{"200": 2, "401": 1, "503": 1}
exit=1
```

Breakdown: the `WARNING …` line went to stderr and the JSON to stdout. That's why in the second run, where stderr is sent to `/dev/null`, the warning is gone but the JSON stays. This is how a script fits into pipelines: `logstat.py access.log | jq .` gets clean JSON. The password is masked before it reaches the log. `sys.exit("5xx found")` prints the text to stderr and returns code 1 — CI or cron will see the failure. `python3 logstat.py --help` shows the help for free.

</details>

## HTTP requests and AWS

### requests
```python
import requests

session = requests.Session()                       # reuses TCP connections — faster with many requests
session.headers["PRIVATE-TOKEN"] = os.environ["GITLAB_TOKEN"]

r = session.get(
    "https://gitlab.com/api/v4/projects",
    params={"membership": "true", "per_page": 100}, # → ?membership=true&per_page=100
    timeout=10,                                     # ALWAYS set a timeout: there's none by default, and a request can hang forever
)
r.raise_for_status()                                # raise an exception on 4xx/5xx — otherwise an error is easy to miss
projects = r.json()

r = session.post(url, json={"name": "x"}, timeout=10)   # json= serialises and sets the Content-Type itself
```

### boto3 — the AWS SDK
```python
import boto3
from datetime import datetime, timedelta, timezone

ec2 = boto3.client("ec2", region_name="us-east-2")
cutoff = datetime.now(timezone.utc) - timedelta(days=30)

# Pagination: describe_* returns a limited number of objects at a time.
# Without a paginator you'll get only the first page and decide there are no old snapshots.
paginator = ec2.get_paginator("describe_snapshots")
old = []
for page in paginator.paginate(OwnerIds=["self"]):
    for snap in page["Snapshots"]:
        if snap["StartTime"] < cutoff:          # StartTime is an aware datetime, and so is cutoff
            old.append(snap["SnapshotId"])

for sid in old:
    if args.dry_run:
        log.info("would delete %s", sid)
    else:
        ec2.delete_snapshot(SnapshotId=sid)
```
**Where boto3 gets credentials from.** It searches by itself along a chain: environment variables (`AWS_ACCESS_KEY_ID`...) → a profile in `~/.aws/config` (including SSO) → a role (IRSA in an EKS pod, an instance profile on EC2, OIDC in CI). That's why keys are never written in code.

**client and resource.** `boto3.client` is low-level, one-to-one with the API. `boto3.resource` is an object wrapper that AWS no longer develops. Use `client`.

### 🧪 Mini task

**Task.** Start a local HTTP server (`python3 -m http.server`) with a `health.json` file and, using `requests` (installed on the fly with `uv run --with requests`) with a timeout, get the version from the JSON, handle a 404 with `raise_for_status()`, and catch a timeout when calling an unreachable address.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo '{"status": "ok", "version": "1.4.2"}' > health.json
python3 -m http.server 8765 --bind 127.0.0.1 >/dev/null 2>&1 &
SERVER=$!
sleep 1
uv run -q --no-project --with requests python - <<'EOF'
import requests

session = requests.Session()
r = session.get("http://127.0.0.1:8765/health.json", timeout=3)
r.raise_for_status()
print(r.status_code, r.json()["version"])

try:
    session.get("http://127.0.0.1:8765/missing", timeout=3).raise_for_status()
except requests.HTTPError as e:
    print("HTTPError:", e.response.status_code)

try:
    session.get("http://10.255.255.1/", timeout=1)
except requests.ConnectTimeout:
    print("ConnectTimeout after 1s")
EOF
kill $SERVER
```

Expected output:

```text
200 1.4.2
HTTPError: 404
ConnectTimeout after 1s
```

Breakdown: without `raise_for_status()` a 404 isn't an error: `requests` would return an object and the script would carry on with empty data. `10.255.255.1` is a non-routable address, packets go "nowhere". Without `timeout=1` the request would hang until the OS timeout, with it you get a clear `ConnectTimeout` after a second. `uv run --with` installs the library into a temporary environment for this run only and leaves nothing behind. A local server stands in for AWS here so the task works without an account. For boto3 the same approach is provided by the `moto` library.

</details>

## Concurrency: the GIL, threads, processes, asyncio

**The task.** Check the availability of 500 URLs. Sequentially, with a 5-second timeout each, that's up to 40 minutes, while the CPU spends almost all the time waiting for the network.

**The GIL (Global Interpreter Lock).** A lock in CPython: at any moment only *one* thread of a process executes Python bytecode. That's why threads don't speed up *computation* (CPU-bound tasks). But while waiting for I/O (network, disk, `sleep`) a thread *releases* the GIL, and other threads run. That's why threads work great for I/O tasks.

| Task type | Example | Solution |
|---|---|---|
| I/O-bound (waiting for network or disk) | polling APIs, SSH to 100 servers, health checks | `ThreadPoolExecutor` or `asyncio` |
| CPU-bound (computing) | parsing gigabytes, compression, hashes | `ProcessPoolExecutor` (several processes — each has its own GIL) |

Almost all DevOps tasks are I/O-bound (they spend most of their time waiting for the network or disk: API calls, SSH, reading files) rather than CPU-bound (limited by computation: compression, parsing gigabytes of JSON). The simplest tool that's good enough is a thread pool:
```python
from concurrent.futures import ThreadPoolExecutor, as_completed
import requests

def check(url):
    try:
        return url, requests.get(url, timeout=5).status_code
    except requests.RequestException as e:
        return url, f"ERROR {type(e).__name__}"

urls = [f"https://service-{i}.example.com/health" for i in range(500)]
with ThreadPoolExecutor(max_workers=50) as pool:          # 50 requests at the same time
    futures = [pool.submit(check, u) for u in urls]       # submit starts a task and returns a Future — a "promise of a result"
    for future in as_completed(futures):                  # get the results as they become ready
        url, status = future.result()
        if status != 200:
            print(url, status)
# 500 URLs × 5 s / 50 threads ≈ 50 seconds in the worst case instead of 40 minutes
```
Or simpler, if the order of results matters: `results = list(pool.map(check, urls))`.

**asyncio** is a different approach: one thread, many tasks that yield control themselves at `await` while waiting. More efficient than threads with thousands of concurrent connections, but it requires async libraries (`aiohttp`, `httpx`, not `requests`), and all the code along the chain becomes `async`:
```python
import asyncio
import httpx

async def check(client, url):
    r = await client.get(url, timeout=5)     # await: "while we wait for the response — run other tasks"
    return url, r.status_code

async def main(urls):
    async with httpx.AsyncClient() as client:
        return await asyncio.gather(*(check(client, u) for u in urls))   # start them all and wait

results = asyncio.run(main(urls))
```
Python 3.13+ has an experimental build without the GIL (free-threaded), but production still uses the ordinary one.

### 🧪 Mini task

**Task.** Simulate checking 20 hosts, each "waiting for the network" for 0.2 seconds (`time.sleep`). Time a sequential pass and a pass through a `ThreadPoolExecutor` with 10 threads, and check that the results are identical and in the same order.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import time
from concurrent.futures import ThreadPoolExecutor

def check(host):
    time.sleep(0.2)                 # simulates waiting for the network
    return host, "ok"

hosts = [f"node-{i}" for i in range(20)]

start = time.perf_counter()
seq = [check(h) for h in hosts]
print(f"sequential: {time.perf_counter() - start:.1f}s")

start = time.perf_counter()
with ThreadPoolExecutor(max_workers=10) as pool:
    par = list(pool.map(check, hosts))
print(f"10 threads: {time.perf_counter() - start:.1f}s")
print("same result:", seq == par)
EOF
```

Expected output:

```text
sequential: 4.1s
10 threads: 0.4s
same result: True
```

Breakdown: 20 × 0.2 s ≈ 4 s sequentially. With 10 threads the hosts go in two "waves" of 10, hence ≈ 0.4 s. The GIL doesn't get in the way because a thread releases it while waiting (`sleep`, network). `pool.map` returns results in input order, so the comparison gives `True`. Your timings may differ by hundredths. If you replace `sleep` with a heavy computation, threads won't speed it up — that needs `ProcessPoolExecutor`.

</details>

## Tests: pytest
**Why in DevOps.** A script that deletes snapshots or edits DNS without tests is a risk. Tests for the data parsing and filtering logic catch errors before production.
```python
# src/mytool/parse.py
def parse_status(line: str) -> int:
    parts = line.split()
    if len(parts) < 9:
        raise ValueError(f"short line: {line!r}")
    return int(parts[8])

# tests/test_parse.py
import pytest
from mytool.parse import parse_status

def test_parse_ok():
    line = '1.1.1.1 - - [12/Oct/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 5'
    assert parse_status(line) == 200          # a plain assert — pytest shows what didn't match what

@pytest.mark.parametrize("line", ["", "garbage", "a b c"])   # one test — many inputs
def test_parse_bad(line):
    with pytest.raises(ValueError):           # check that the function raises an exception
        parse_status(line)

def test_reads_env(monkeypatch, tmp_path):    # pytest fixtures are passed by argument name
    monkeypatch.setenv("ENV", "test")         # override an environment variable for this test only
    cfg = tmp_path / "cfg.yaml"               # a temporary folder deleted after the test
    cfg.write_text("replicas: 2")
```
Running: `pytest -q` (brief), `pytest -k parse` (only tests with "parse" in the name), `pytest -x` (stop at the first failure). External calls are replaced with mocks: `unittest.mock.patch`, and for AWS — the `moto` library (a fake AWS in memory).

### 🧪 Mini task

**Task.** Put `parse_status` into `parse.py`, and in `test_parse.py` write a test for a valid access.log line and a parametrized test for three broken lines. Run pytest via `uv run --with pytest` without installing it globally.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > parse.py <<'EOF'
def parse_status(line: str) -> int:
    parts = line.split()
    if len(parts) < 9:
        raise ValueError(f"short line: {line!r}")
    return int(parts[8])
EOF
cat > test_parse.py <<'EOF'
import pytest
from parse import parse_status

def test_ok():
    assert parse_status('1.1.1.1 - - [12/Oct/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 5') == 200

@pytest.mark.parametrize("line", ["", "garbage", "a b c"])
def test_bad(line):
    with pytest.raises(ValueError):
        parse_status(line)
EOF
uv run -q --no-project --with pytest pytest -q
```

Expected output:

```text
....                                                                     [100%]
4 passed in 0.01s
```

Breakdown: 4 tests — one `test_ok` and three `test_bad` variants from `parametrize`; each dot is a passed test. Try replacing the date in the test with a short `[x]`: the test fails with `assert 5 == 200`. Without the space inside the date the line splits into 8 fields instead of 9, and the response size lands where the status should be. A great example of why tests should run on *real* log lines.

</details>

## Practice tasks (typical live coding)
DevOps interviews usually ask not for algorithms but for data processing: a log, JSON, a config. Talk through the solution out loud, start with a simple working version, then improve it.

**1. The top 10 IPs in access.log and the share of 5xx.**
```python
from collections import Counter

ips = Counter()
total = errors = 0
with open("access.log", encoding="utf-8", errors="replace") as f:
    for line in f:                       # line by line — the file may be huge
        parts = line.split()
        if len(parts) < 9:               # skip broken lines rather than failing
            continue
        total += 1
        ips[parts[0]] += 1               # the first field is the client IP
        if parts[8].startswith("5"):     # the ninth field is the HTTP status
            errors += 1

for ip, count in ips.most_common(10):
    print(f"{ip:15} {count}")
if total:
    print(f"5xx: {errors / total:.2%}")
```
What can be discussed next: how to speed it up (read in parallel chunks — it's CPU-bound, so processes), how to handle compressed logs (`gzip.open`), how to do the same in bash (`awk '{print $1}' | sort | uniq -c | sort -rn | head`).

**2. Find pods with more than N restarts.**
```python
import json, subprocess

out = subprocess.run(["kubectl", "get", "pods", "-A", "-o", "json"],
                     capture_output=True, text=True, check=True).stdout
for pod in json.loads(out)["items"]:
    meta = pod["metadata"]
    # containerStatuses may be missing for Pending pods — hence .get with an empty list
    for cs in pod["status"].get("containerStatuses", []):
        if cs["restartCount"] > 3:
            print(f'{meta["namespace"]}/{meta["name"]} {cs["name"]}: {cs["restartCount"]}')
```

**3. Compare two configs: what was added, removed, changed.**
```python
def diff(old: dict, new: dict) -> dict:
    return {
        "added":   {k: new[k] for k in new.keys() - old.keys()},   # keys only in new
        "removed": {k: old[k] for k in old.keys() - new.keys()},   # keys only in old
        "changed": {k: (old[k], new[k]) for k in old.keys() & new.keys() if old[k] != new[k]},
    }

diff({"a": 1, "b": 2}, {"b": 3, "c": 4})
# → {'added': {'c': 4}, 'removed': {'a': 1}, 'changed': {'b': (2, 3)}}
```
Three techniques at once can be seen here: sets of keys (`keys()` supports `-` and `&`), a dict comprehension, and a tuple for a pair of values.

**4. Checking brackets** (the classic stack task):
```python
def balanced(s: str) -> bool:
    pairs = {")": "(", "]": "[", "}": "{"}
    stack = []
    for ch in s:
        if ch in "([{":
            stack.append(ch)                       # push an opening bracket onto the stack
        elif ch in pairs:
            if not stack or stack.pop() != pairs[ch]:
                return False                       # the closing bracket didn't match the last opening one
    return not stack                               # all opened brackets must be closed

balanced("{[()]}")   # → True
balanced("{[(])}")   # → False
```

**5. Two sum** — find two numbers that add up to target. Naively — two nested loops, O(n²). With a dictionary — one pass, O(n):
```python
def two_sum(nums, target):
    seen = {}                          # number → its index
    for i, n in enumerate(nums):
        if target - n in seen:         # has the needed "pair" already appeared?
            return seen[target - n], i
        seen[n] = i
    return None

two_sum([2, 7, 11, 15], 9)   # → (0, 1)
```
This is exactly the optimisation (a dict or set instead of a nested loop) that is expected from you.

**6. An operational tool: auditing a host's settings.**
A typical Platform/SRE task: check that a host is really configured as intended (isolated cores, THP, hugepages, interrupt affinity) and return an error code, so the script can be plugged into an AMI check or CI. The techniques shown here: `argparse`, `pathlib`, parsing the `/proc` and `/sys` formats, clear output plus JSON for machines, the exit code. The `--root` parameter lets you test the script on a fake file tree, without a real server.
```python
#!/usr/bin/env python3
"""Checks that the host is configured for low latency. Exit code 1 if anything deviates."""
import argparse
import json
import sys
from pathlib import Path


def parse_cpus(text: str) -> set[int]:
    """'2-5,8' -> {2, 3, 4, 5, 8}"""
    cpus = set()
    for part in text.strip().split(","):
        if not part:
            continue
        if "-" in part:
            start, end = part.split("-")
            cpus.update(range(int(start), int(end) + 1))
        else:
            cpus.add(int(part))
    return cpus


def read(root: Path, path: str) -> str:
    return (root / path.lstrip("/")).read_text().strip()


def check(root: Path, expected_isolated: set[int], min_hugepages: int) -> list[dict]:
    results = []

    def add(name, ok, actual, expected):
        results.append({"check": name, "ok": ok, "actual": actual, "expected": expected})

    isolated = parse_cpus(read(root, "/sys/devices/system/cpu/isolated"))
    add("isolated_cpus", isolated == expected_isolated, sorted(isolated), sorted(expected_isolated))

    thp = read(root, "/sys/kernel/mm/transparent_hugepage/enabled")
    mode = thp[thp.index("[") + 1:thp.index("]")]          # "always [madvise] never" -> "madvise"
    add("thp", mode in ("madvise", "never"), mode, "madvise|never")

    meminfo = dict(line.split(":", 1) for line in read(root, "/proc/meminfo").splitlines())
    hugepages = int(meminfo["HugePages_Total"])
    add("hugepages", hugepages >= min_hugepages, hugepages, f">={min_hugepages}")

    numa_balancing = read(root, "/proc/sys/kernel/numa_balancing")
    add("numa_balancing", numa_balancing == "0", numa_balancing, "0")

    # No interrupt may be handled on the isolated cores
    bad_irqs = []
    for affinity in sorted((root / "proc/irq").glob("*/smp_affinity_list")):
        if parse_cpus(affinity.read_text()) & expected_isolated:
            bad_irqs.append(affinity.parent.name)
    add("irq_affinity", not bad_irqs, bad_irqs, "no IRQs on isolated cores")

    return results


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--isolated", required=True, help="the expected isolated cores, e.g. 2-7")
    parser.add_argument("--min-hugepages", type=int, default=0)
    parser.add_argument("--root", default="/", help="the filesystem root (for tests)")
    parser.add_argument("--json", action="store_true", help="JSON output for CI")
    args = parser.parse_args()

    results = check(Path(args.root), parse_cpus(args.isolated), args.min_hugepages)
    if args.json:
        print(json.dumps(results, ensure_ascii=False, indent=2))
    else:
        for r in results:
            print(f"{'OK  ' if r['ok'] else 'FAIL'} {r['check']}: {r['actual']} (expected {r['expected']})")
    return 0 if all(r["ok"] for r in results) else 1


if __name__ == "__main__":
    sys.exit(main())
```
```bash
sudo ./host_check.py --isolated 2-7 --min-hugepages 1024
# → OK   isolated_cpus: [2, 3, 4, 5, 6, 7] (expected [2, 3, 4, 5, 6, 7])
# → OK   thp: never (expected madvise|never)
# → OK   hugepages: 1024 (expected >=1024)
# → OK   numa_balancing: 0 (expected 0)
# → FAIL irq_affinity: ['26'] (expected no IRQs on isolated cores)
echo $?   # → 1 — such a host won't pass the check in the image pipeline
```
What to discuss in an interview: how to test it (pytest with `tmp_path`, where a fake `/proc` tree is created), how to add new checks without rewriting `main` (a list of check functions), why the exit is non-zero on any error, and how to expose the result to Prometheus (node_exporter's textfile collector).

**More for practising on your own:**
- a script that deletes EBS snapshots older than N days, with `--dry-run` and logging;
- a health check of a list of URLs with parallelism and a final report;
- a CLI for the GitLab API: a list of failed pipelines over the last day across all of a group's projects (with pagination);
- your own Prometheus exporter on `prometheus_client` that exposes a metric from some API.

## Live coding: the same tasks in Python

These are the same 11 mini tasks as in the Bash topic (section "Live coding: text-processing tasks"), but solved in Python. Interviews often let you choose the language — it helps to know both versions and to explain when each is better: a Bash pipeline is shorter for a one-off task in a terminal, Python is clearer once conditions, CSV, JSON and error handling appear.

**How to solve out loud** — the same as in Bash: clarify the input format and edge cases, start with a simple working version, check it on the example, then improve. Use clear variable names — the interviewer reads the code in real time.

**Setup.** Create the same files as for Bash: paste the "Setup" block from the Bash topic (section "Live coding: text-processing tasks") into a terminal. Each solution below is a standalone script: save it, for example, as `t.py` in the same directory and run `python3 t.py`.

### 1. The third word of each line — on one line

**Task.** From `lines.txt` print the third word of each line on one line separated by spaces; skip lines with fewer than three words.

```text
the quick brown fox
jumps over the lazy dog
a b c
only two
```

Expected output: `brown the c`.

<details class="qa"><summary>Solution and walkthrough</summary>

```python
with open("lines.txt") as f:
    thirds = [line.split()[2] for line in f if len(line.split()) >= 3]
print(" ".join(thirds))
```

```text
brown the c
```

- `for line in f` reads the file line by line — it does not load the whole file into memory, which matters for big logs.
- `line.split()` **without an argument** splits on any run of spaces, tabs and newlines and drops empty pieces. `[2]` is the third element (indexes start at zero).
- `if len(line.split()) >= 3` is a filter in the list comprehension: short lines are skipped, otherwise `[2]` would fail with `IndexError`.
- `" ".join(thirds)` joins the list of strings with spaces. An alternative is `print(*thirds)`: the star unpacks the list into separate arguments, and `print` puts a space between them by default.

The solution calls `split()` twice. To split the line once, use the assignment expression operator `:=` (Python 3.8+):

```python
with open("lines.txt") as f:
    thirds = [w[2] for line in f if len(w := line.split()) >= 3]
```

**Common mistake:** `line.split(" ")` with an explicit space. It splits on **every** space and keeps empty strings: `"  a   b  c".split(" ")` gives `['', '', 'a', '', '', 'b', '', 'c']`, and `[2]` returns `a`. Without an argument — `['a', 'b', 'c']`. This is exactly the same trap as `cut -d' '` in Bash.

</details>

### 2. The two most frequent words

**Task.** In `words.txt` find the two most frequent words ignoring case and punctuation, and print them with their counts.

```text
Deploy api, deploy web.
Rollback; deploy API!
restart db
```

<details class="qa"><summary>Solution and walkthrough</summary>

```python
import re
from collections import Counter
from pathlib import Path

text = Path("words.txt").read_text().lower()
words = re.findall(r"[a-z]+", text)
for word, count in Counter(words).most_common(2):
    print(count, word)
```

```text
3 deploy
2 api
```

- `Path(...).read_text()` reads the whole file into a string — for a small file this is simpler than a loop.
- `.lower()` — everything to lower case so that `Deploy` and `deploy` match.
- `re.findall(r"[a-z]+", text)` returns a list of all continuous runs of Latin letters — punctuation and spaces simply do not get into the result. For Cyrillic and any other alphabet: `r"[^\W\d_]+"` — "a letter of any alphabet" (`\w` without digits and `_`).
- `Counter(words)` is a "word → how many times" dictionary. `most_common(2)` returns a list of `(word, count)` pairs in descending order.

**Common mistakes:**
- **`text.split()` instead of a regular expression** — `api,` and `API!` keep their punctuation and are counted as different words.
- **A hand-made dictionary instead of `Counter`** — `if w not in d: d[w] = 0` works, but in an interview you are expected to know `Counter` (or at least `dict.get(w, 0) + 1`).

Likely follow-up: "on equal counts — alphabetically" → `sorted(Counter(words).items(), key=lambda p: (-p[1], p[0]))[:2]`: the minus before the count gives descending order, the second tuple element gives alphabetical order on a tie.

</details>

### 3. How many requests per HTTP status

**Task.** From `access.log` (nginx combined format) count the requests per HTTP status and the share of 5xx.

<details class="qa"><summary>Solution and walkthrough</summary>

```python
from collections import Counter

statuses = Counter()
with open("access.log") as f:
    for line in f:
        parts = line.split()
        if len(parts) > 8:               # skip a broken line instead of crashing
            statuses[parts[8]] += 1      # the ninth field (index 8) is the status
for status, count in statuses.most_common():
    print(status, count)

errors = sum(c for s, c in statuses.items() if s.startswith("5"))
total = sum(statuses.values())
if total:
    print(f"5xx: {errors} of {total} ({errors / total:.1%})")
```

```text
200 2
500 1
302 1
404 1
502 1
5xx: 2 of 6 (33.3%)
```

- A `Counter()` can be incremented like a normal dictionary: for a new key the starting value is 0, there is no `KeyError`.
- `most_common()` without an argument returns all elements in descending order; on equal counts the order of first appearance is kept.
- `sum(c for s, c in ...)` is a generator expression: it sums the counts only for statuses starting with `5`.
- `f"{errors / total:.1%}"` is the percent format: it multiplies by 100, keeps one decimal place and adds `%`.
- `if total:` protects against division by zero on an empty file.

**Common mistake — a broken request line.** If the log has `"-"` instead of `"GET / HTTP/1.1"`, the fields shift and `parts[8]` is no longer the status. It is more reliable to split on quotes: the status is the first word after the second quote.

```python
status = line.split('"')[2].split()[0]
```

For the line `... "-" 400 0 "-" "-"` the `split()` version gives `"-"`, and the quote version gives `400`.

</details>

### 4. Remove duplicates while keeping the order

**Task.** Print the lines of `services.txt` (`web api web db api`) without duplicates, in the order of first appearance.

<details class="qa"><summary>Solution and walkthrough</summary>

```python
from pathlib import Path

services = Path("services.txt").read_text().split()
print(" ".join(dict.fromkeys(services)))
```

```text
web api db
```

- `dict.fromkeys(list)` creates a dictionary whose keys are the list elements. Dictionary keys are unique, and since Python 3.7 a dictionary **keeps insertion order** — duplicates are dropped, the order of first appearance remains.
- `" ".join(...)` iterates over the dictionary keys and joins them.

**Common mistake:** `set(services)` also removes duplicates, but a set does not guarantee order. If the interviewer asks to "keep the order", this is exactly what is being checked.

A version worth being able to write by hand (sometimes asked "without built-in tricks"):

```python
seen, result = set(), []
for s in services:
    if s not in seen:          # a set lookup is O(1), in a list it would be O(n)
        seen.add(s)
        result.append(s)
```

</details>

### 5. What is in one list but not in the other

**Task.** `all.txt` has all hosts, `done.txt` has the ones already updated. Print the ones not updated, in the original order.

<details class="qa"><summary>Solution and walkthrough</summary>

```python
from pathlib import Path

done = set(Path("done.txt").read_text().split())
todo = [h for h in Path("all.txt").read_text().split() if h not in done]
print(" ".join(todo))
```

```text
web-2 cache-1 api-1
```

- `done` is a set: the check `h not in done` takes O(1) regardless of the list size.
- The list comprehension walks `all.txt` in order, so the order is kept.
- `.split()` without an argument also removes empty lines and the `\r` of Windows files — a common problem of the `grep` version does not arise here.

**Why not just set subtraction:** `set(all_hosts) - done` gives the right set but in arbitrary order, for example `{'cache-1', 'api-1', 'web-2'}`.

**Why not a list for `done`:** `h not in done_list` checks the elements one by one — O(n) per check, O(n·m) in total. On two lists of 100,000 hosts this is minutes instead of a fraction of a second. This "list → set" switch is exactly what the interviewer wants to hear.

</details>

### 6. Average and maximum response time

**Task.** In `times.txt` the fourth field is the response time. Print the number of requests, the average and the slowest request with its path.

<details class="qa"><summary>Solution and walkthrough</summary>

```python
from pathlib import Path

rows = [line.split() for line in Path("times.txt").read_text().splitlines() if line.strip()]
times = [float(r[3]) for r in rows]
slowest = max(rows, key=lambda r: float(r[3]))
print(f"requests={len(times)} avg={sum(times) / len(times):.3f} max={slowest[3]} ({slowest[1]})")
```

```text
requests=4 avg=0.500 max=1.500 (/api)
```

- `splitlines()` splits the text into lines, `if line.strip()` drops empty ones (for example, the last one after the final newline).
- `float(r[3])` — the field is read as a string, and it has to be turned into a number for arithmetic. Without it `max` compares strings as text: `"9.5"` ends up greater than `"10.0"`, because text is compared character by character and the character `9` is greater than the character `1`.
- `max(rows, key=...)` returns the **whole row** with the largest key value — so the path `slowest[1]` is available right away.

**Common mistake:** division by zero on an empty file. In a real script check `if times:` before computing the average. There is a `statistics` module for statistics: `statistics.mean(times)`, `statistics.median(times)`.

</details>

### 7. Filtering a CSV

**Task.** From `users.csv` (`name,email,age`) print the name and email of everyone older than 30.

<details class="qa"><summary>Solution and walkthrough</summary>

```python
import csv

with open("users.csv", newline="") as f:
    for row in csv.DictReader(f):
        if int(row["age"]) > 30:
            print(row["name"], row["email"])
```

```text
alice alice@example.com
carol carol@example.com
```

- `csv.DictReader` takes the first line as the header and returns every following line as a dictionary `{"name": ..., "email": ..., "age": ...}` — access by column name reads better than `row[2]`.
- `int(row["age"])` — the `csv` module returns all values as strings; without `int()` the comparison `"34" > 30` fails with `TypeError`.
- `newline=""` is what the documentation recommends for `csv`: the module handles line breaks itself, including those inside quoted values.

**The main advantage over awk:** `csv` correctly parses values with a comma inside quotes. The line `"Smith, John",j@example.com,45` becomes `['Smith, John', 'j@example.com', '45']`, while `awk -F,` would split the name into two fields.

</details>

### 8. Reverse a string and the word order

**Task.** For `hello world` print the string backwards, the words in reverse order, and each word reversed.

<details class="qa"><summary>Solution and walkthrough</summary>

```python
s = "hello world"
print(s[::-1])
print(" ".join(s.split()[::-1]))
print(" ".join(w[::-1] for w in s.split()))
```

```text
dlrow olleh
world hello
olleh dlrow
```

- `s[::-1]` is a slice with step −1: from the end to the start. It works for strings, lists and tuples.
- `s.split()[::-1]` — first a list of words, then that list backwards.
- In the third line the generator reverses each word, while the word order stays the same.

**Common mistake:** `s.reverse()` — strings have no such method, strings are immutable. Lists do have `reverse()`, but it changes the list in place and returns `None`, so `print(lst.reverse())` prints `None`. Another option for a string is `"".join(reversed(s))`: `reversed` returns an iterator that has to be joined back.

</details>

### 9. The largest files in a directory

**Task.** Find the three largest files inside `data/` including subdirectories.

<details class="qa"><summary>Solution and walkthrough</summary>

```python
from pathlib import Path

files = [p for p in Path("data").rglob("*") if p.is_file()]
for p in sorted(files, key=lambda p: p.stat().st_size, reverse=True)[:3]:
    print(f"{p.stat().st_size / 1024 / 1024:6.1f} MB  {p}")
```

```text
   2.9 MB  data/logs/app.log
   1.9 MB  data/logs/old.log
   1.0 MB  data/dump.sql
```

- `rglob("*")` is a recursive walk: all files and directories at any depth. `is_file()` keeps only files.
- `p.stat().st_size` is the size in bytes. `sorted(..., key=..., reverse=True)` — descending, `[:3]` — the first three.
- `:6.1f` in an f-string is a number 6 characters wide with one decimal place, to keep the column aligned.

**What to improve if asked:**
- On a real server the walk fails on a directory without access permissions — wrap `stat()` in `try/except OSError` and skip such files.
- For huge trees there is no need to sort everything: `heapq.nlargest(3, files, key=...)` keeps only the top three in memory.
- `is_file()` follows symbolic links — one file can be counted twice; if that matters, check `p.is_symlink()`.

</details>

### 10. Change values in a config

**Task.** In `app.conf` set `port=8080`, `debug=false`, and if there is no `timeout` key — append `timeout=30`. Leave the other lines (including comments) untouched.

<details class="qa"><summary>Solution and walkthrough</summary>

```python
from pathlib import Path


def set_keys(path, updates):
    path = Path(path)
    lines, seen = [], set()
    for line in path.read_text().splitlines():
        key = line.split("=", 1)[0].strip()
        if key in updates:
            line = f"{key}={updates[key]}"
            seen.add(key)
        lines.append(line)
    lines += [f"{k}={v}" for k, v in updates.items() if k not in seen]
    path.write_text("\n".join(lines) + "\n")


set_keys("app.conf", {"port": "8080", "debug": "false", "timeout": "30"})
print(Path("app.conf").read_text(), end="")
```

```text
host=0.0.0.0
port=8080
debug=false
timeout=30
```

- `line.split("=", 1)` — split at most once: the value `url=http://a?b=c` does not fall apart into extra pieces. `[0].strip()` is the key without surrounding spaces.
- Lines whose key is not in `updates` (including `# ...` comments) are added as is — the order and comments are kept.
- `seen` remembers which keys have already been replaced; the rest are appended at the end. A second run changes nothing — the function is idempotent.
- `print(..., end="")` — the file already ends with a newline, a second one is not needed.

**What to discuss:**
- **`configparser`** fits INI files with `[section]` sections, but loses comments when writing — that is why for "as is" edits people often write their own parsing, as above.
- **Atomic write:** if the process dies in the middle of `write_text`, the file is left truncated. The reliable way is to write to a temporary file next to it and rename with `os.replace(tmp, path)`: a rename within one file system is atomic.

</details>

### 11. FizzBuzz — a warm-up

**Task.** Print the numbers from 1 to 15 on one line: multiples of 3 — `Fizz`, multiples of 5 — `Buzz`, multiples of both — `FizzBuzz`.

<details class="qa"><summary>Solution and walkthrough</summary>

```python
def fizzbuzz(n):
    if n % 15 == 0:
        return "FizzBuzz"
    if n % 3 == 0:
        return "Fizz"
    if n % 5 == 0:
        return "Buzz"
    return str(n)


print(" ".join(fizzbuzz(i) for i in range(1, 16)))
```

```text
1 2 Fizz 4 Buzz Fizz 7 8 Fizz Buzz 11 Fizz 13 14 FizzBuzz
```

- A separate function is easy to test: `assert fizzbuzz(15) == "FizzBuzz"` — interviewers like that.
- `range(1, 16)` is 1 to 15: the right bound is not included.
- `return str(n)` — `join` joins only strings, the number has to be converted.

A short version sometimes shown "for elegance":

```python
print(" ".join("Fizz" * (n % 3 == 0) + "Buzz" * (n % 5 == 0) or str(n) for n in range(1, 16)))
```

`True` and `False` in arithmetic are 1 and 0, so `"Fizz" * True` is `"Fizz"` and `"Fizz" * False` is an empty string. If both parts are empty, `or` substitutes the number. In an interview it is better to write the clear version first and show the short one as an option.

</details>

## Interview questions

**Which types are mutable and which aren't? Why does it matter?**
Immutable: `int`, `float`, `bool`, `str`, `tuple`, `frozenset`, `bytes`. Mutable: `list`, `dict`, `set`, class instances. It matters for three reasons: a mutable object passed into a function changes outside too; only hashable (immutable) objects can be dictionary keys; a mutable default value in a function is shared between calls.

**How are arguments passed — by value or by reference?**
Neither: "by object reference" (call by sharing). The function gets a reference to the same object. If it *mutates* a mutable object (`lst.append`), it's visible outside. If it *rebinds* the name (`lst = [...]`), nothing changes outside: only the local label changed.

**What's the trap with `def f(x=[])`?**
The default value is evaluated once when the function is defined, and all calls share one list. The right way: `x=None`, and inside `if x is None: x = []`.

**What's the difference between list, tuple, set and dict?**
A list is ordered and mutable, accessed by index. A tuple is ordered and immutable, hashable (can be a dict key). A set holds unique elements, has a fast `in` and set operations. A dict maps key → value with O(1) lookup by key. An `in` lookup in a list is O(n), in a set/dict O(1).

**`is` versus `==`?**
`==` compares values, `is` checks that it's one and the same object in memory. `is` is used only with `None`.

**What is a generator and why is it needed?**
An object that yields values one at a time on demand (a function with `yield` or an expression in parentheses). It doesn't keep the whole sequence in memory: you can process a file of tens of gigabytes or an infinite stream. It can be iterated once.

**What is a decorator? An example from practice?**
A function that takes a function and returns a wrapped one with extra behaviour. `@decorator` is shorthand for `func = decorator(func)`. Examples: retry with exponential backoff for API calls, timing, logging calls, caching (`@lru_cache`), permission checks.

**What is the GIL? How do you speed up a script that polls 500 servers?**
The GIL is a CPython lock because of which only one thread executes bytecode at a time. But while waiting for I/O a thread releases the GIL. Polling servers is an I/O-bound task, so `ThreadPoolExecutor` or `asyncio` will do. CPU-bound tasks need processes (`ProcessPoolExecutor`).

**What are `*args` and `**kwargs`?**
`*args` collects extra positional arguments into a tuple, `**kwargs` collects extra keyword arguments into a dictionary. They're used in wrappers and decorators to accept any arguments and pass them on.

**How does a shallow copy differ from a deep one?**
`copy.copy(x)` / `lst.copy()` / `lst[:]` create a new outer container, but the nested objects stay shared: a change to a nested list is visible in both copies. `copy.deepcopy(x)` copies everything recursively.

**Why `if __name__ == "__main__":`?**
The block runs only when the file is run directly, not when it's imported. That way a file can be both a script and a module: its functions can be imported into tests or other scripts.

**How do you run shell commands safely from Python?**
`subprocess.run([...])` with a list of arguments, `check=True`, `timeout=...`, `capture_output=True, text=True`. Don't use `shell=True` with external data (shell injection), and don't use `os.system`.

**What's dangerous about `yaml.load`?**
YAML can describe the creation of Python objects, and `yaml.load` with an unsafe loader can execute code from the file. Use `yaml.safe_load`.

**How do you process a file that doesn't fit in memory?**
Read it line by line (`for line in f`), build the processing on generators, read fixed-size blocks (`f.read(1024 * 1024)`), and for JSON Lines or CSV — streaming reads. No whole-file `read()` or `readlines()`.

**A list comprehension or an ordinary loop?**
A comprehension — for simply building a collection: shorter and faster. An ordinary loop — when the logic is complex, there are several conditions or side effects (writes, API calls).

**How do you organise a Python project's dependencies?**
A separate venv (or uv) per project, dependencies in `pyproject.toml`, pinned versions in a lock file or in `requirements.txt` with `==`, `.venv` in `.gitignore`, no `sudo pip install` into the system Python.

**How do you make a script convenient to operate?**
`argparse` with `--help` and `--dry-run`, `logging` instead of `print`, clear exit codes, configuration via environment variables and arguments (no hard-coding), timeouts on all network calls, retries with backoff, idempotency (re-running doesn't break anything), tests for the logic.

**Why is `0.1 + 0.2 != 0.3`?**
A float is stored in binary, and 0.1 is an infinite fraction in binary, so it's stored approximately. Compare floats with `math.isclose`, and use `Decimal` for money.

**What will this code print and why?**
```python
funcs = [lambda: i for i in range(3)]
print([f() for f in funcs])
```
`[2, 2, 2]`, not `[0, 1, 2]`. The lambda remembers not the *value* of `i` but the *variable* `i` (late binding), and by the time of the call the loop has finished and `i == 2`. The fix: `lambda i=i: i` — the default value is evaluated when the lambda is created.
