# Bash

A thorough Bash primer: from how the shell parses a command in the first place to functions, arrays and robust scripts for CI and cron. Each topic: what it is, why, how it works, an example with explanations and typical mistakes.

How to read: run the examples in a terminal. The result is shown in a comment after `# →`. The system bash on macOS is very old (3.2), so for the examples with associative arrays install a new one: `brew install bash`.

## What a shell is and when you need Bash specifically

A **shell** is a program that reads commands (from a terminal or a script file), runs other programs and connects them together. **Bash** is the most common shell on Linux. There are others: `sh` (the POSIX standard; in Debian/Ubuntu it's `dash`, in Alpine — `ash` from busybox), `zsh` (the default on macOS), `fish`.

Bash's main strength is **gluing programs together**. Run `kubectl`, filter the output through `grep`, count with `wc`, stop on an error. It's ideal for this: short, and available on every server, in every container and on every CI runner.

The weaknesses: everything is a string, there are no proper data structures, it's easy to make mistakes with quotes and spaces, and error handling is awkward. A practical rule: **if a script has grown beyond 100–200 lines, works with JSON or APIs, or has complex logic — write it in Python.**

**Bash and sh are not the same thing.** If the first line of a script is `#!/bin/sh`, in an Alpine container it will be run by `ash`, and in Debian by `dash` (minimalist shells that support only the POSIX standard — the set of shell features common to all Unix systems). They don't have `[[ ]]`, arrays, `$'...'`, `{1..5}` and much else from these notes. If you use bash features, write `#!/usr/bin/env bash`.

### 🧪 Mini task

**Task.** Compare the system `/bin/bash` on macOS with the bash from `PATH` (after `brew install bash`): print the versions and run two bash 4+ features in each — uppercasing with `${env^^}` and an associative array.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
for sh in /bin/bash bash; do
  echo "== $sh: $($sh -c 'echo $BASH_VERSION')"
  $sh -c 'env=prod; echo "upper: ${env^^}"; declare -A ports=([api]=8080); echo "api -> ${ports[api]}"' 2>&1
done
```

Expected output:

```text
== /bin/bash: 3.2.57(1)-release
/bin/bash: upper: ${env^^}: bad substitution
== bash: 5.3.20(1)-release
upper: PROD
api -> 8080
```

Breakdown: the system bash 3.2 fails right at `${env^^}` with `bad substitution` and never reaches the array. The new bash runs both lines. The practical takeaway: a script that works for you may not work on another machine or in Alpine (`ash`), which is why the `#!/usr/bin/env bash` shebang and checking the version matter. If your second line also shows 3.2, the brew bash is not first in your `PATH`.

</details>

## How the shell parses a command — the foundation of everything

Almost all of Bash's "oddities" are explained by *what* the shell does to a line and *in what order* before running the command. Simplified:

1. **Splitting into words and operators** by whitespace, taking quotes into account: `echo "a b" c` is the command `echo` with two arguments: `a b` and `c`.
2. **Expansions**, in this order:
   - braces: `file{1,2}.txt` → `file1.txt file2.txt`;
   - tilde: `~` → `/home/alex`;
   - variables: `$name`, `${name}`;
   - command substitution: `$(date)` → the command's output;
   - arithmetic: `$((2 + 3))` → `5`.
3. **Word splitting** — the result of expansions that *isn't quoted* is split on spaces, tabs and newlines (the characters in the `IFS` variable — Internal Field Separator; it can be redefined, for example `IFS=,` to parse CSV).
4. **Filename expansion (globbing)** — words with `*`, `?`, `[...]` are replaced by a list of matching files.
5. **Quote removal**, and the command is run with the resulting arguments.

Steps 3 and 4 are the source of most bugs:
```bash
file="my report.txt"
touch "$file"         # created a file with a space in its name

rm $file              # without quotes: after step 3 there are TWO arguments: "my" and "report.txt"
                      # → rm: cannot remove 'my': No such file or directory
rm "$file"            # in quotes: step 3 doesn't apply, one argument — the file is deleted

msg="Error: *"
echo $msg             # without quotes * expands into the list of files in the current folder!
echo "$msg"           # → Error: *
```
Hence **the main rule of Bash: always put variables and command substitutions in double quotes** — `"$var"`, `"$(cmd)"`. They're written without quotes only when word splitting is wanted deliberately.

### 🧪 Mini task

**Task.** Using an `args` function that prints the number of arguments and each argument in brackets, see what the shell turns a variable with a space and a variable with `*` into — unquoted and quoted. Then try to delete the file `my report.txt` with an unquoted and a quoted command.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
bash <<'EOF'
args() { printf '%s args:' "$#"; printf ' [%s]' "$@"; echo; }
file="my report.txt"
touch "$file" a.log b.log
args $file
args "$file"
msg="Error: *.log"
args $msg
args "$msg"
rm $file
rm "$file" && ls -1
EOF
```

Expected output:

```text
2 args: [my] [report.txt]
1 args: [my report.txt]
3 args: [Error:] [a.log] [b.log]
1 args: [Error: *.log]
rm: my: No such file or directory
rm: report.txt: No such file or directory
a.log
b.log
```

Breakdown: unquoted, `$file` split into 2 arguments (word splitting), and `$msg` also expanded into the list of `*.log` files (globbing). Quoted, both stayed a single argument. That is why `rm $file` tried to delete two non-existent files `my` and `report.txt`, while `rm "$file"` removed the right one. Imagine the same with `rm -rf $dir/` — that is exactly how incidents happen.

</details>

## The exit code: all the logic rests on it

Every command returns an **exit code** when it finishes — a number from 0 to 255:
- `0` — success;
- any other number — an error (which one is up to the program: `grep` returns 1 if it found nothing and 2 on an error).

This is the opposite of the usual logic in programming languages, where 0 means "false". In the shell **0 means "true", i.e. success**.

```bash
ls /etc > /dev/null
echo $?              # → 0   — $? holds the last command's exit code
ls /nope 2> /dev/null
echo $?              # → 2   — an error
```
`if`, `while`, `&&` and `||` check exactly the exit code, not the "text" of the output:
```bash
if grep -q "ERROR" app.log; then     # -q: print nothing, just return the code
  echo "the log contains errors"
fi

mkdir -p /tmp/build && cd /tmp/build     # && — run the second only if the first succeeded
cd /nope || exit 1                       # || — run the second only if the first failed
```
A script's exit code is the code of its last command or the value from `exit N`. CI, cron, systemd and Kubernetes look exactly at it to understand whether everything succeeded.

### 🧪 Mini task

**Task.** Get all three exit codes of `grep -q`: found, not found, error (no file). Check `&&` and `||` on a successful and a failing command. Then find out the exit code of the `false | true` pipe and how to get the code of each of its parts.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
bash <<'EOF'
printf 'INFO start\nERROR disk full\n' > app.log
grep -q ERROR app.log; echo "found: $?"
grep -q FATAL app.log; echo "not found: $?"
grep -q ERROR nope.log 2>/dev/null; echo "no file: $?"
mkdir -p build && echo "mkdir ok"
cd /nope 2>/dev/null || echo "cd failed, handled"
false | true; echo "pipe without pipefail: $?"
false | true; echo "PIPESTATUS: ${PIPESTATUS[*]}"
EOF
```

Expected output:

```text
found: 0
not found: 1
no file: 2
mkdir ok
cd failed, handled
pipe without pipefail: 0
PIPESTATUS: 1 0
```

Breakdown: `grep` tells "not found" (1) apart from "error" (2) — in a script that matters: a missing line is a normal situation, a missing file is an incident. A pipe's exit code is that of the *last* command, so `false | true` gives 0 and the error is lost. The `PIPESTATUS` array keeps the codes of all parts. In scripts people usually enable `set -o pipefail` instead.

</details>

## Your first script: the shebang and running it
```bash
#!/usr/bin/env bash
echo "Hello, $USER. Today is $(date +%F)"
```
```bash
chmod +x hello.sh     # give execute permission
./hello.sh            # run — the script executes in a NEW bash process
bash hello.sh         # the same, but the interpreter is given explicitly (the shebang is ignored)
```
The **shebang** (`#!`) is the first line, which tells the kernel which program to run the file with. `#!/usr/bin/env bash` looks for `bash` in `PATH`. It's more portable than `#!/bin/bash`: on macOS with brew bash or on NixOS, bash isn't in `/bin`.

### 🧪 Mini task

**Task.** Create `hello.sh` with the `#!/usr/bin/env bash` shebang that prints `$0`, the bash version and the arguments. Try running it with `./hello.sh` before `chmod +x`, after it, and with `bash hello.sh`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > hello.sh <<'EOF'
#!/usr/bin/env bash
echo "hello from $0, bash ${BASH_VERSINFO[0]}, args: $*"
EOF
./hello.sh 2>/dev/null || echo "without chmod: exit=$?"
chmod +x hello.sh
./hello.sh one two
bash hello.sh three
```

Expected output:

```text
without chmod: exit=126
hello from ./hello.sh, bash 5, args: one two
hello from hello.sh, bash 5, args: three
```

Breakdown: without the execute permission the kernel refuses; code 126 means "command found but cannot be executed" (127 is "command not found"). After `chmod +x` the kernel reads the shebang and starts `bash` from `PATH` — hence version 5. `bash hello.sh` doesn't need `chmod`, because the file is read by an already running bash. `$0` shows exactly how the script was invoked.

</details>

## Variables

```bash
name="web"                 # assignment: NO spaces around =
name = "web"               # an error: bash looks for a command "name" with the arguments "=" and "web"
echo "$name"               # → web
echo "${name}-01"          # → web-01 — curly braces separate the name from the text
echo "$name-01"            # → web-01 — works here, because '-' can't be part of a name
echo "$name_01"            # → (empty) — bash looks for a variable name_01, which doesn't exist
```
Variables are **global** for the whole script by default and **all strings**. An empty and a non-existent variable both expand to an empty string (unless `set -u` is on).

```bash
readonly VERSION="1.2.0"   # a constant: trying to change it gives an error
unset name                 # delete a variable
```

### Environment variables and export
An ordinary variable is visible only to the current shell. For it to be seen by **programs started from it** (child processes), it has to be exported into the environment:
```bash
REGION=us-east-2
bash -c 'echo "the child sees: $REGION"'   # → the child sees:      — empty
export REGION
bash -c 'echo "the child sees: $REGION"'   # → the child sees: us-east-2

AWS_PROFILE=prod aws s3 ls     # a variable for ONE command only, it won't stay in the current shell
env | grep AWS                 # look at the environment
```
That's why `export AWS_PROFILE=...` in a terminal affects all the following `aws` commands.

### Command substitution
`$(command)` is replaced with what the command printed to stdout (the trailing newline is trimmed):
```bash
today=$(date +%F)                       # → 2026-10-12
pod_count=$(kubectl get pods --no-headers | wc -l)
echo "Pods: $pod_count"
latest=$(ls -t /backups | head -1)
```
The old syntax is backticks `` `date` ``. It works the same, but backticks are awkward to nest and easy to confuse with single quotes. Use `$(...)`.

### Quotes: three kinds
```bash
name="Alex"
echo "Hello, $name. Today is $(date +%A)"   # double: variables and $(...) are expanded,
                                            # but there's NO word splitting or globbing
echo 'Hello, $name'                         # single: everything literal → Hello, $name
echo $'line1\nline2\ttab'                   # $'...': escape sequences (\n, \t)
echo "Price: \$5, quote: \""                # escaping inside double quotes — with a backslash
```
You can't put a single quote inside single quotes, not even with `\`. You have to close the string, insert the quote and reopen it: `'it'\''s'` → `it's`.

### Special variables
| Variable | What it contains |
|---|---|
| `$0` | the script's name (as it was called) |
| `$1`, `$2`, … `${10}` | positional arguments (from the 10th — only in braces) |
| `$#` | the number of arguments |
| `"$@"` | all the arguments, **each as a separate word**. Needed almost always |
| `"$*"` | all the arguments joined into **one** string with spaces |
| `$?` | the exit code of the last command |
| `$$` | the PID of the current shell (handy for unique temporary file names) |
| `$!` | the PID of the last process started in the background with `&` |
| `$LINENO` | the current line number of the script (for error messages) |
| `$FUNCNAME`, `$BASH_SOURCE` | the current function's name and the path to the script file |

The difference between `"$@"` and `"$*"`, by example. The script `args.sh`:
```bash
#!/usr/bin/env bash
echo "Arguments: $#"
for a in "$@"; do echo "[$a]"; done
echo "---"
for a in "$*"; do echo "[$a]"; done
```
```bash
./args.sh "my file" second
# → Arguments: 2
# → [my file]
# → [second]
# → ---
# → [my file second]       — everything got joined into one string
```
If a wrapper script says `kubectl $@` without quotes, the argument `"my file"` falls apart into two. The right way is `kubectl "$@"`.

### 🧪 Mini task

**Task.** Check when a child process sees a variable: without `export`, after `export`, and with an assignment for a single command. Show the `$name_01` vs `${name}_01` trap. Write `args.sh` that iterates over `"$@"` and `"$*"` and run it with an argument containing a space.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
bash <<'EOF'
REGION=us-east-2
bash -c 'echo "child sees: [$REGION]"'
export REGION
bash -c 'echo "child sees: [$REGION]"'
PROFILE=prod bash -c 'echo "one-shot: [$PROFILE]"'
echo "after one-shot: [${PROFILE:-}]"
name=web
echo "[$name_01] [${name}_01]"
cat > args.sh <<'EOT'
echo "count: $#"
for a in "$@"; do echo "@ [$a]"; done
for a in "$*"; do echo "* [$a]"; done
EOT
bash args.sh "my file" second
EOF
```

Expected output:

```text
child sees: []
child sees: [us-east-2]
one-shot: [prod]
after one-shot: []
[] [web_01]
count: 2
@ [my file]
@ [second]
* [my file second]
```

Breakdown: a plain variable lives only in the current shell; the child `bash -c` doesn't see it until it is exported. The `VAR=x cmd` form passes the variable to one command and doesn't keep it in the shell. `$name_01` is a variable called `name_01`, which doesn't exist, hence empty. `"$@"` kept `my file` as one argument, while `"$*"` glued everything into one string — that's why wrappers use `"$@"`.

</details>

## Parameter expansion: working with strings without sed and cut

Bash can process a variable's value right during substitution. This is faster than running `sed`, `basename` or `cut` (no process is created), and it works even in minimal containers without utilities.

```bash
path="/var/log/nginx/access.log.gz"
```
**Cut off the beginning** — `#` (the shortest match) and `##` (the longest). The pattern is like a glob, `*` means "anything":
```bash
echo "${path#*/}"     # → var/log/nginx/access.log.gz — removed the shortest "something/" at the start
echo "${path##*/}"    # → access.log.gz — removed the longest "something/" = basename
```
**Cut off the end** — `%` (short) and `%%` (long):
```bash
echo "${path%/*}"     # → /var/log/nginx — removed the shortest "/something" at the end = dirname
echo "${path%.gz}"    # → /var/log/nginx/access.log — removed the .gz extension
echo "${path%%.*}"    # → /var/log/nginx/access — removed everything from the FIRST dot
```
A mnemonic: on the keyboard `#` is to the left of `$`, and `%` is to the right. `#` cuts from the left, `%` from the right.

**Replacement, length, substring, case:**
```bash
echo "${path/nginx/apache}"   # → /var/log/apache/access.log.gz — replace the first occurrence
echo "${path//\//_}"          # → _var_log_nginx_access.log.gz — replace ALL '/' with '_'
echo "${#path}"               # → 28 — the string's length
echo "${path:0:4}"            # → /var — a substring: from position 0, length 4
echo "${path: -6}"            # → log.gz — the last 6 characters (the space before the minus is mandatory)
env=prod
echo "${env^^}"               # → PROD — to upper case (bash 4+); ${env,,} — to lower case
```

**Default values and checks** — used all the time in CI scripts:
```bash
echo "${REGION:-us-east-2}"      # if REGION is empty or unset — substitute us-east-2
echo "${REPLICAS:=1}"            # the same, but also ASSIGN REPLICAS=1
: "${TOKEN:?TOKEN must be set}"  # if empty — print an error and exit the script
                                 # ':' is an empty command, needed only to trigger the expansion
args="${DEBUG:+--verbose}"       # substitute --verbose only if DEBUG is NOT empty
```

A practical example — rename all `.jpeg` files to `.jpg`:
```bash
for f in *.jpeg; do
  mv -- "$f" "${f%.jpeg}.jpg"    # -- means "what follows are file names, not options"
done
```

### 🧪 Mini task

**Task.** For the path `/var/log/nginx/access.log.gz` get the basename, dirname, the path without `.gz`, the extension and the length without external tools. Rename all `*.jpeg` to `*.jpg` (one name has a space). Give `REGION` a default and make `TOKEN` mandatory.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
bash <<'EOF'
path="/var/log/nginx/access.log.gz"
echo "basename: ${path##*/}"
echo "dirname:  ${path%/*}"
echo "no .gz:   ${path%.gz}"
echo "ext:      ${path##*.}"
echo "length:   ${#path}"
touch a.jpeg "b c.jpeg"
for f in *.jpeg; do mv -- "$f" "${f%.jpeg}.jpg"; done
ls -1
echo "region: ${REGION:-us-east-2}"
bash -c ': "${TOKEN:?TOKEN is required}"' 2>&1
EOF
```

Expected output:

```text
basename: access.log.gz
dirname:  /var/log/nginx
no .gz:   /var/log/nginx/access.log
ext:      gz
length:   28
a.jpg
b c.jpg
region: us-east-2
bash: line 1: TOKEN: TOKEN is required
```

Breakdown: `##*/` strips the longest "something/" from the left — that's the basename; `%/*` strips the shortest "/something" from the right — that's the dirname; `##*.` leaves the extension. The loop with `"$f"` and `--` correctly renamed the file with a space too. `${TOKEN:?…}` ended the process with a clear error — in CI you immediately see which variable is missing instead of a failure somewhere later.

</details>

## Arithmetic

Bash works only with **integers**. Arithmetic is written in `$(( ))` (return a value) or `(( ))` (evaluate and get an exit code):
```bash
a=7; b=2
echo $(( a + b ))      # → 9   — inside $(( )) you can omit the $ before variables
echo $(( a / b ))      # → 3   — integer division, the fractional part is dropped
echo $(( a % b ))      # → 1   — the remainder
echo $(( 2 ** 10 ))    # → 1024

(( count = a * 3 ))    # assignment
(( count++ ))          # increment
if (( count > 20 )); then echo "a lot"; fi   # comparison in the familiar mathematical notation
```
Fractional calculations — via `awk` or `bc`:
```bash
awk "BEGIN { printf \"%.2f\n\", 10 / 3 }"   # → 3.33
echo "scale=2; 10 / 3" | bc                 # → 3.33
```
**The `set -e` trap:** `(( expr ))` returns code 1 if the expression's result is 0. So `x=0; (( x++ ))` returns 1 (the postfix `++` returns the *old* value, i.e. 0), and a script with `set -e` silently exits. Safe variants: `x=$((x + 1))` or `(( ++x ))` (when starting from 0).

### 🧪 Mini task

**Task.** Compute disk usage percentage for `used=37`, `total=50` with integers — first correctly, then in the "wrong" order of operations, then with decimals via `awk`. Raise an alert above 70%. And check the `(( x++ ))` trap with `x=0` under `set -e`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
bash <<'EOF'
used=37; total=50
echo "integer: $(( used * 100 / total ))%"
echo "wrong:   $(( used / total * 100 ))%"
awk -v u=$used -v t=$total 'BEGIN { printf "float:   %.1f%%\n", u * 100 / t }'
if (( used * 100 / total > 70 )); then echo "disk alert"; fi
bash -c 'set -e; x=0; (( x++ )); echo "never printed"'; echo "x++ under set -e exit code: $?"
bash -c 'set -e; x=0; x=$((x + 1)); echo "safe increment: x=$x"'
EOF
```

Expected output:

```text
integer: 74%
wrong:   0%
float:   74.0%
disk alert
x++ under set -e exit code: 1
safe increment: x=1
```

Breakdown: bash computes only in integers, so `37 / 50` is 0 and `0 * 100` is 0 too. Multiply before dividing. A fractional result needs `awk` (or `bc`). The trap: `(( x++ ))` returns the *old* value 0, and `(( 0 ))` is exit code 1, so `set -e` silently ends the script: the `never printed` line didn't appear. `x=$((x + 1))` always returns 0.

</details>

## Conditions

### if and checking the exit code
`if` runs a command and looks at its exit code. Square brackets aren't part of the `if` syntax but a test *command*:
```bash
if ping -c1 -W2 10.0.0.1 > /dev/null; then   # any command
  echo "reachable"
elif [[ -f /etc/maintenance ]]; then          # the [[ ]] test command
  echo "maintenance mode"
else
  echo "unreachable" >&2                       # >&2 — print to stderr
fi
```

### `[ ]` versus `[[ ]]`
- `[` is an ordinary command (a synonym for `test`; there's a file `/usr/bin/[`), part of POSIX. Its arguments go through word splitting and globbing, so quotes are mandatory: `[ -n $var ]` with an empty `var` turns into `[ -n ]` and gives *true*.
- `[[` is a built-in Bash construct. Inside it there's no word splitting, and `&&`, `||`, pattern matching and regular expressions work. **In bash scripts, use `[[ ]]`.**

```bash
var=""
[ -n $var ] && echo "not empty"      # → not empty (!) — a bug due to the missing quotes
[[ -n $var ]] && echo "not empty"    # → (nothing) — correct
```

### Test operators
| Test | Meaning |
|---|---|
| `-e path` | exists (a file, a folder — anything) |
| `-f path` | exists and is a regular file |
| `-d path` | exists and is a directory |
| `-L path` | a symbolic link |
| `-s path` | the file exists and isn't empty |
| `-r`, `-w`, `-x` | read / write / execute permission |
| `a -nt b` | file a is newer than file b |
| `-z "$s"` | the string is empty |
| `-n "$s"` | the string isn't empty |
| `"$a" == "$b"`, `"$a" != "$b"` | the strings are equal / not equal |
| `"$a" < "$b"` | lexicographically less (only in `[[ ]]`) |
| `-eq -ne -lt -le -gt -ge` | comparing **numbers** in `[ ]` and `[[ ]]` |
| `-v VAR` | the variable is set (even if empty) |

A common mistake is comparing numbers as strings: `[[ 10 < 9 ]]` is true, because the comparison is character by character ("1" is less than "9"). For numbers — `-lt` or `(( 10 < 9 ))`.

### Patterns and regular expressions in `[[ ]]`
```bash
branch="feature/JIRA-123-login"
if [[ $branch == feature/* ]]; then       # a glob pattern: the right-hand side WITHOUT quotes
  echo "a feature branch"
fi

tag="v1.24.3"
if [[ $tag =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then   # a regular expression
  major=${BASH_REMATCH[1]}                # the capture groups are in the BASH_REMATCH array
  minor=${BASH_REMATCH[2]}
  echo "major=$major minor=$minor"        # → major=1 minor=24
else
  echo "the tag doesn't look like semver" >&2
fi
```
If you put the pattern on the right in quotes (`== "feature/*"`), it becomes an ordinary string, and the comparison is literal.

### Logical operators
```bash
[[ -f $cfg && -r $cfg ]]       # AND — inside [[ ]]
[[ $env == prod || $env == production ]]   # OR
[[ ! -d $dir ]]                # NOT
cmd1 && cmd2                   # between commands
```

### case — choosing by pattern
More convenient than a chain of `elif` when you compare one value with several options:
```bash
case "$env" in
  prod|production)              # | — several options
    replicas=3 ;;               # ;; — the end of a branch
  stage)
    replicas=2 ;;
  dev*|feature-*)               # glob patterns
    replicas=1 ;;
  *)                            # everything else (default)
    echo "unknown environment: $env" >&2
    exit 1 ;;
esac
```
`case` is the standard way to parse a script's subcommand (`./deploy.sh start|stop|status`).

### 🧪 Mini task

**Task.** For the paths `dir`, `file`, `run.sh` (executable) and `missing` determine the type with `[[ -d ]]`, `[[ -x ]]`, `[[ -f ]]`. Parse the versions `1.24.3`, `v1.30.0`, `latest` with a regex in `[[ =~ ]]` and extract major and minor. Use `case` to split the environments `prod`, `stage-2`, `dev`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
bash <<'EOF'
mkdir dir; touch file; touch run.sh; chmod +x run.sh
for p in dir file run.sh missing; do
  if [[ -d $p ]]; then kind=directory
  elif [[ -x $p ]]; then kind=executable
  elif [[ -f $p ]]; then kind=file
  else kind="does not exist"; fi
  echo "$p: $kind"
done
for v in 1.24.3 v1.30.0 latest; do
  if [[ $v =~ ^v?([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    echo "$v -> major=${BASH_REMATCH[1]} minor=${BASH_REMATCH[2]}"
  else
    echo "$v -> not semver"
  fi
done
for env in prod stage-2 dev; do
  case "$env" in
    prod|production) echo "$env: require approval" ;;
    stage*)          echo "$env: auto deploy" ;;
    *)               echo "$env: skip" ;;
  esac
done
EOF
```

Expected output:

```text
dir: directory
file: file
run.sh: executable
missing: does not exist
1.24.3 -> major=1 minor=24
v1.30.0 -> major=1 minor=30
latest -> not semver
prod: require approval
stage-2: auto deploy
dev: skip
```

Breakdown: the order of checks matters — `run.sh` is also a regular file, so `-x` is tested before `-f`. Regex groups land in the `BASH_REMATCH` array: `[1]` is major, `[2]` is minor; the `v?` prefix makes the `v` optional. The regex is not quoted, otherwise it becomes a plain string. `case` patterns are globs: `stage*` caught `stage-2`, and `*)` acts as the default.

</details>

## Loops

### for over a list
```bash
for env in dev stage prod; do
  echo "Deploying to $env"
done

for host in web1 web2 web3; do
  ssh "$host" 'uptime'            # a command in single quotes runs on the remote side
done
```

### for over files — via a glob, not via ls
```bash
shopt -s nullglob                 # if there are no files, the pattern expands to nothing
for f in /var/log/app/*.log; do
  gzip -- "$f"
done
```
**Why not `for f in $(ls *.log)`.** The output of `ls` goes through word splitting, and a file `my app.log` turns into two iterations: `my` and `app.log`. A glob returns file names as ready separate words, so spaces in them don't matter.

**Why `nullglob`.** Without it, if there are no `*.log` files, the loop runs once with the value being the literal string `/var/log/app/*.log`, and `gzip` fails. With `nullglob` the loop simply doesn't run.

### for over numbers
```bash
for i in {1..5}; do echo "$i"; done          # → 1 2 3 4 5
for i in {0..20..5}; do echo "$i"; done      # → 0 5 10 15 20 — with a step
for i in {01..03}; do echo "node-$i"; done   # → node-01 node-02 node-03 — with leading zeros

n=5
for i in {1..$n}; do echo "$i"; done         # → {1..5} — WON'T work: braces are expanded
                                             #   BEFORE variables are substituted (see the expansion order)
for (( i = 1; i <= n; i++ )); do echo "$i"; done   # this works — a C-style loop
for i in $(seq 1 "$n"); do echo "$i"; done         # or this (the external seq utility)
```

### while read — reading a file line by line
The most common loop in scripts. The correct form:
```bash
while IFS= read -r line; do
  echo "Line: $line"
done < hosts.txt
```
What's important here:
- `read` reads one line from stdin into a variable and returns 0 as long as there are lines;
- `< hosts.txt` after `done` redirects the file into **the whole loop**;
- `-r` — don't treat a backslash as an escape (otherwise `C:\new` turns into `C:new`);
- `IFS=` — don't trim spaces at the start and end of the line.

The last line is lost if the file doesn't end with a newline (read returns an error even though it read the data). The reliable variant: `while IFS= read -r line || [[ -n $line ]]; do`.

Reading by fields — `read` spreads a line across several variables, splitting by `IFS`:
```bash
# /etc/passwd: user:x:uid:gid:comment:home:shell
while IFS=: read -r user _ uid _ _ home shell; do   # _ — "throw away" an unneeded field
  if (( uid >= 1000 )); then
    echo "$user -> $home ($shell)"
  fi
done < /etc/passwd
```

### The trap: a pipe into while loses variables
```bash
count=0
kubectl get pods -o name | while read -r pod; do
  (( ++count ))
done
echo "$count"       # → 0 (!)
```
**Why.** Each part of a pipe runs in a separate copy of the shell (a subshell). The loop incremented `count` in the copy, and the copy died together with the pipe.

**The solution — process substitution `<(...)`.** The command's output is fed into the loop as a file, and the loop runs in the current shell:
```bash
count=0
while IFS= read -r pod; do
  (( ++count ))
done < <(kubectl get pods -o name)    # the space between < and <( is mandatory
echo "$count"       # → the correct number
```

### until, break, continue — waiting and retries
```bash
# until — "repeat WHILE the condition is false" (the opposite of while)
until kubectl rollout status deploy/api --timeout=10s; do
  echo "still rolling out..."
done

# a retry with a limited number of attempts and a growing pause
for attempt in {1..10}; do
  if curl -fsS http://localhost:8080/health > /dev/null; then
    echo "the service is up"
    break                              # exit the loop
  fi
  if (( attempt == 10 )); then
    echo "didn't come up in 10 attempts" >&2
    exit 1
  fi
  sleep $(( attempt * 2 ))             # 2, 4, 6... seconds
done

# continue — skip an iteration
while IFS= read -r host; do
  [[ -z $host || $host == \#* ]] && continue   # skip empty lines and comments
  ping -c1 -W1 "$host" > /dev/null || echo "$host is unreachable"
done < hosts.txt
```
`curl` flags: `-f` — return an error on HTTP 4xx/5xx (without it curl treats any server response as success), `-s` — no progress bar, `-S` — but still show errors.

### 🧪 Mini task

**Task.** From `hosts.txt` (name, IP, state) print `ssh` commands line by line for running hosts only. Count the file's lines two ways — with `cat | while` and with `while … < file` — and compare the final counter. Write an `until` loop that waits for a flag file to appear.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
bash <<'EOF'
printf 'web-1 10.0.0.1 running\nweb-2 10.0.0.2 stopped\ndb 10.0.0.9 running\n' > hosts.txt
while read -r name ip state; do
  [[ $state == running ]] && echo "ssh $ip  # $name"
done < hosts.txt
count=0
cat hosts.txt | while read -r _; do (( ++count )); done
echo "after pipe: count=$count"
count=0
while read -r _; do (( ++count )); done < hosts.txt
echo "after redirect: count=$count"
attempt=0
until [[ -f ready.flag ]]; do
  (( ++attempt ))
  echo "attempt $attempt: not ready"
  (( attempt == 2 )) && touch ready.flag
done
echo "ready after $attempt attempts"
EOF
```

Expected output:

```text
ssh 10.0.0.1  # web-1
ssh 10.0.0.9  # db
after pipe: count=0
after redirect: count=3
attempt 1: not ready
attempt 2: not ready
ready after 2 attempts
```

Breakdown: `read -r name ip state` splits the line into fields on whitespace. The counter after the pipe stayed 0: each part of a pipe runs in a subshell, and the variable changed in a copy that then vanished. With the `< hosts.txt` redirect the loop runs in the current shell, and the counter is 3. `until` loops while the condition is false — a typical readiness wait.

</details>

## Functions

### Declaration and arguments
```bash
greet() {
  local name="$1"                  # a function's arguments are also $1, $2, "$@", $#
  local greeting="${2:-Hello}"     # the second argument with a default value
  echo "$greeting, $name!"
}

greet "Alex"                       # → Hello, Alex!
greet "Alex" "Hi"                  # → Hi, Alex!
```
A function is called **like a command**: without parentheses, with arguments separated by spaces. Inside a function `$1`, `$2`, `$#`, `"$@"` refer to its arguments, not to the script's. A function must be declared *above* the place where it's called: bash reads the script top to bottom.

### local — mandatory
Variables in a function are **global** by default, and the function can quietly corrupt a script variable:
```bash
i=100
count_files() {
  for i in *; do :; done        # i without local — overwrites the global i
}
count_files
echo "$i"                       # → the name of the last file, not 100
```
The rule: **declare every variable inside a function with `local`.**

### How a function "returns" a value
In Bash `return` returns only an **exit code** (0–255), not data. For data there are three ways.

**1. Print to stdout and capture with `$(...)`** — the most common:
```bash
get_pod_ip() {
  kubectl get pod "$1" -o jsonpath='{.status.podIP}'
}
ip=$(get_pod_ip api-0)
echo "IP: $ip"
```
A consequence: everything the function prints to stdout ends up in the result. That's why **logs and messages inside such functions go to stderr** (`echo "..." >&2`), otherwise they get mixed up with the return value.

**2. The exit code for "yes/no"** — the function is used directly in an `if`:
```bash
is_port_open() {
  nc -z -w 2 "$1" "$2" 2> /dev/null    # the function's exit code = the last command's code
}

if is_port_open db.internal 5432; then
  echo "the database is reachable"
fi

has_tool() {
  command -v "$1" > /dev/null 2>&1 || { echo "$1 is required" >&2; return 1; }
}
has_tool kubectl || exit 1
```

**3. Writing into a variable by name (nameref, bash 4.3+)** — without a subshell, handy for several values:
```bash
parse_image() {
  local -n _repo=$2 _tag=$3        # _repo and _tag are references to the caller's variables
  _repo="${1%:*}"
  _tag="${1##*:}"
}
parse_image "nginx:1.27" repo tag
echo "$repo / $tag"                # → nginx / 1.27
```

### Example: a generic retry function
```bash
retry() {
  local max="$1" delay="$2"
  shift 2                          # drop the first two arguments — what's left is the command itself
  local attempt=1
  until "$@"; do                   # "$@" — run the passed command with all its arguments
    if (( attempt >= max )); then
      echo "failed after $attempt attempts: $*" >&2
      return 1
    fi
    echo "attempt $attempt failed, waiting ${delay}s" >&2
    sleep "$delay"
    (( ++attempt ))
  done
}

retry 5 3 curl -fsS https://example.com/health
retry 3 10 helm upgrade --install api ./chart -n prod
```
Three techniques are visible here: `shift` (shifting the arguments), `"$@"` for passing the whole command while preserving its arguments, messages to stderr.

### Function libraries
Shared functions are moved into a separate file and included with `source` (or a dot `.`):
```bash
# lib.sh
log() { printf '%s [%s] %s\n' "$(date +%T)" "$1" "${*:2}" >&2; }

# deploy.sh
source "$(dirname "$0")/lib.sh"
log INFO "starting the deployment"
```
`source` executes the file *in the current shell*, so its functions and variables become available.

### 🧪 Mini task

**Task.** Write a `retry N command…` function that repeats the command up to N times and returns 1 if all attempts fail. Test it on a function that succeeds on the third call, and on `false`. Separately, return a string from a function via `echo` and `$(…)`, and make sure a `local` variable isn't visible outside.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
bash <<'EOF'
retry() {
  local times=$1 n=1; shift
  until "$@"; do
    (( n >= times )) && { echo "giving up after $n tries: $*" >&2; return 1; }
    echo "try $n failed: $*" >&2
    (( ++n ))
  done
}
flaky() { (( ++CALLS >= 3 )); }
version() { local v="1.24.3"; echo "${v%.*}"; }
CALLS=0
retry 5 flaky && echo "flaky ok after $CALLS calls"
retry 2 false || echo "retry returned $?"
minor=$(version)
echo "minor=$minor v=[${v:-}]"
EOF
```

Expected output:

```text
try 1 failed: flaky
try 2 failed: flaky
flaky ok after 3 calls
try 1 failed: false
giving up after 2 tries: false
retry returned 1
minor=1.24 v=[]
```

Breakdown: `"$@"` after `shift` is the command itself with its arguments, so `retry` works with any command. Attempt messages go to stderr so they don't mix with useful output. A function "returns" a number only via the exit code (`return 1`) and data via stdout, captured with `$(…)`. `v` is declared `local`, so it is empty outside.

</details>

## Arrays

An ordinary variable holds one string. An array is a list of strings. The main benefit: **storing elements with spaces and building command arguments without problems with quotes.**

### Indexed arrays
```bash
pods=(api web "db primary")        # three elements, the third has a space
pods+=(cache)                      # append to the end

echo "${pods[0]}"                  # → api           — an element by index (from 0)
echo "${pods[-1]}"                 # → cache         — the last one
echo "${#pods[@]}"                 # → 4             — the number of elements
echo "${!pods[@]}"                 # → 0 1 2 3       — the list of indexes
echo "${pods[@]:1:2}"              # → web db primary — a slice: from index 1, two elements

for p in "${pods[@]}"; do          # iteration: "${arr[@]}" MUST be in quotes
  echo "[$p]"
done
# → [api] [web] [db primary] [cache]   — "db primary" stayed one element

unset 'pods[1]'                    # delete an element (the indexes don't shift!)
```
`$pods` without an index is only the first element. A common mistake.

### Reading into an array
```bash
mapfile -t nodes < <(kubectl get nodes -o name)   # each output line is an element; -t strips the \n
echo "Nodes: ${#nodes[@]}"
mapfile -t hosts < hosts.txt                      # from a file
```

### Building a command's arguments — the main practical technique
**The problem:** you need to add flags to a command depending on conditions. With a string this breaks on spaces and quotes:
```bash
opts="--set image.tag=$TAG --set 'annotations.note=hello world'"
helm upgrade api ./chart $opts     # quotes inside a string DON'T work, 'hello and world' fall apart
```
**The solution — an array:**
```bash
args=(upgrade --install api ./chart --namespace "$NS" --set "image.tag=$TAG")
[[ $DRY_RUN == true ]] && args+=(--dry-run)
[[ -n $VALUES ]] && args+=(-f "$VALUES")
args+=(--set "annotations.note=hello world")      # the space inside the element is preserved

helm "${args[@]}"                                  # each element is a separate argument
```

### Associative arrays (dictionaries, bash 4+)
```bash
declare -A replicas=([api]=3 [web]=2)   # declare -A is mandatory
replicas[db]=1

echo "${replicas[api]}"                 # → 3
for svc in "${!replicas[@]}"; do        # ${!arr[@]} — the list of KEYS
  echo "$svc -> ${replicas[$svc]}"
done
[[ -v replicas[api] ]] && echo "the api key exists"
```
The order of keys during iteration isn't guaranteed. The macOS system bash 3.2 doesn't support them: `declare: -A: invalid option`.

### 🧪 Mini task

**Task.** Build the `kubectl get pods -n prod` command in an array, adding `-l` with a selector containing a space and the `--dry-run=client` flag only when conditions hold, and check how many arguments you get. Read a file into an array with `mapfile` and count repeats with an associative array.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
bash <<'EOF'
args() { printf '%s args:' "$#"; printf ' [%s]' "$@"; echo; }
ns=prod; dry_run=true; selector="app=web, tier=front"
cmd=(kubectl get pods -n "$ns")
[[ -n $selector ]] && cmd+=(-l "$selector")
$dry_run && cmd+=(--dry-run=client)
args "${cmd[@]}"
printf 'api\nweb\napi\ndb\napi\n' > pods.txt
mapfile -t pods < pods.txt
echo "read ${#pods[@]} lines, last: ${pods[-1]}"
declare -A seen
for p in "${pods[@]}"; do (( ++seen[$p] )); done
for p in $(printf '%s\n' "${!seen[@]}" | sort); do echo "$p=${seen[$p]}"; done
EOF
```

Expected output:

```text
8 args: [kubectl] [get] [pods] [-n] [prod] [-l] [app=web, tier=front] [--dry-run=client]
read 5 lines, last: api
api=3
db=1
web=1
```

Breakdown: `"${cmd[@]}"` expands into exactly 8 arguments, and the `app=web, tier=front` selector stayed one despite the space. With a string instead of an array it would have split into two. `mapfile -t` strips newlines, `${pods[-1]}` is the last element (bash 4.3+). `${!seen[@]}` are the associative array's keys; their order isn't guaranteed, so they are sorted for output.

</details>

## Input and output: streams and redirections

Every process has three standard streams:
- **0 — stdin** (input), the keyboard by default;
- **1 — stdout** (normal output), the terminal by default;
- **2 — stderr** (errors and diagnostics), also the terminal by default.

Why two output streams: so that data (stdout) can be passed along a pipe or into a file, while errors and logs (stderr) remain visible to a human.

```bash
cmd > out.txt           # stdout to a file (the file is OVERWRITTEN)
cmd >> out.txt          # append stdout to the end of the file
cmd 2> err.txt          # stderr to a file
cmd > all.log 2>&1      # both streams into one file
cmd &> all.log          # the same, shorter (bash only)
cmd > /dev/null 2>&1    # discard all output (/dev/null is a "black hole")
cmd 2> /dev/null        # hide only the errors
cmd < input.txt         # stdin from a file
echo "error" >&2        # print to stderr — this is how error messages are written in scripts
```

**Why the order in `> file 2>&1` matters.** Redirections are applied left to right, and `2>&1` means "send stderr to where stdout points *right now*":
```bash
cmd > file 2>&1    # 1) stdout → file  2) stderr → the same place as stdout, i.e. file. Both are in the file.
cmd 2>&1 > file    # 1) stderr → where stdout points now, i.e. the TERMINAL  2) stdout → file.
                   #    The errors stayed in the terminal.
```

### Pipes
```bash
cmd1 | cmd2            # stdout cmd1 → stdin cmd2
cmd1 2>&1 | cmd2       # both stdout and stderr into the pipe (or cmd1 |& cmd2)
cmd | tee run.log      # tee: show on screen AND write to a file (tee -a — append)
```
An example — the top 5 IPs from an nginx log, step by step:
```bash
awk '{print $1}' access.log |   # take the first field — the IP
  sort |                        # sort so that identical ones end up next to each other
  uniq -c |                     # collapse repeats, counting them (uniq only works with adjacent lines!)
  sort -rn |                    # by number, descending
  head -5                       # the first 5
```

### Here-document — multi-line input right in the script
```bash
cat > /etc/app/config.ini <<EOF
[server]
host=$HOST
port=${PORT:-8080}
generated=$(date +%F)
EOF
```
Everything between `<<EOF` and the `EOF` line is fed to the command's stdin, and variables and `$(...)` are expanded. The closing `EOF` must be at the start of a line without spaces.

If you put the marker in quotes (`<<'EOF'`), there's no substitution. Needed when you write a script in which `$` must stay literal:
```bash
cat > /usr/local/bin/hello <<'EOF'
#!/usr/bin/env bash
echo "Hello, $USER"        # $USER is expanded when hello RUNS, not now
EOF
```

Applying a manifest straight from a script:
```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${NAMESPACE}
  labels:
    owner: ${TEAM}
EOF
```
`-f -` means "read the manifest from stdin".

**Here-string** `<<<` — feed one string to stdin:
```bash
grep -q "Running" <<< "$status"
read -r major minor patch <<< "${version//./ }"    # "1.24.3" → three variables
```

### Process substitution — a command's output as a file
`<(cmd)` substitutes the path to a temporary "file" from which the command's output can be read. Needed when a program expects files specifically:
```bash
diff <(kubectl get cm app -n stage -o yaml) <(kubectl get cm app -n prod -o yaml)
comm -13 <(sort expected.txt) <(sort actual.txt)    # lines that exist only in actual
```

### 🧪 Mini task

**Task.** A function writes `data` to stdout and `oops` to stderr. Compare `> file 2>&1` with `2>&1 > file`: what lands in the file and what goes on into the pipe. Print data on screen and into a file at once with `tee`. Compare a here-document with an unquoted and a quoted marker. Use process substitution and `comm` to find the extra and the missing service.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
bash <<'EOF'
cmd() { echo "data"; echo "oops" >&2; }
cmd > both.txt 2>&1
cmd 2>&1 > out.txt | sed 's/^/via pipe: /'
echo "both.txt: $(tr '\n' ' ' < both.txt)| out.txt: $(cat out.txt)"
cmd 2>/dev/null | tee copy.txt
PORT=9090
cat <<EOT
port=$PORT date_cmd=$(echo now)
EOT
cat <<'EOT'
port=$PORT date_cmd=$(echo now)
EOT
printf 'api\nweb\ndb\n' > expected.txt
printf 'web\napi\nworker\n' > actual.txt
echo "unexpected: $(comm -13 <(sort expected.txt) <(sort actual.txt))"
echo "missing:    $(comm -23 <(sort expected.txt) <(sort actual.txt))"
EOF
```

Expected output:

```text
via pipe: oops
both.txt: data oops | out.txt: data
data
port=9090 date_cmd=now
port=$PORT date_cmd=$(echo now)
unexpected: worker
missing:    db
```

Breakdown: with `> file 2>&1` both streams went to the file. With `2>&1 > file` stderr was first pointed where stdout was *at that moment* (the pipe), and only then stdout went to the file: so `oops` arrived in the pipe and the file has only `data`. The unquoted here-document expanded `$PORT` and `$(…)`; with `'EOT'` the text stayed literal. `comm -13` gives lines only in the second file, `-23` only in the first; it needs sorted input, and `<(sort …)` provides it without temp files.

</details>

## Grouping commands, subshells, background jobs

```bash
{ echo "start"; date; echo "end"; } > report.txt      # a group in the CURRENT shell; the spaces and ; are mandatory
( cd /tmp/build && make )                             # a group in a SUBSHELL: cd won't affect the script
pwd                                                   # → still the original directory
```
A **subshell** is a copy of the current shell in a separate process. Everything that changed in it (variables, `cd`, `set`) disappears after it finishes. Subshells are created by: `( ... )`, `$( ... )`, every part of a pipe, a command with `&`.

### The `a && b || c` trap
It looks like if/else, but it isn't:
```bash
check && echo "ok" || echo "fail"
```
If `check` succeeds but `echo "ok"` fails (for example, stdout is closed), `echo "fail"` runs as well. `c` runs when `a` **or** `b` fails. For real branching — `if`.

### Background jobs and parallelism
```bash
long_backup &                  # & — run in the background, the script continues
backup_pid=$!                  # the background process's PID
do_other_work
wait "$backup_pid"             # wait for a specific process to finish
echo "backup exit code: $?"    # wait returns the background job's exit code

# poll many hosts in parallel, but no more than 4 at a time
for host in "${hosts[@]}"; do
  ssh "$host" uptime > "out-$host.txt" 2>&1 &
  if (( $(jobs -rp | wc -l) >= 4 )); then   # how many background jobs are running now
    wait -n                                  # wait until ANY one finishes (bash 4.3+)
  fi
done
wait                           # wait for all the remaining ones
```
An alternative — `xargs -P`:
```bash
xargs -P 8 -I{} curl -fsS -o /dev/null -w "%{http_code} {}\n" {} < urls.txt
# -P 8: 8 processes at once; -I{}: substitute the line in place of {}
```
More useful things: `timeout 30 cmd` (kill it if it runs longer than 30 seconds), `nohup cmd > out.log 2>&1 &` (the process survives the terminal being closed).

### 🧪 Mini task

**Task.** Show that `cd` inside `( … )` doesn't change the current directory, while `{ …; }` writes the output of several commands into one file. Start three 1-second background jobs (one exits with code 7), wait for each with `wait PID`, print its code and the total time. Compare with `xargs -P 4`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
bash <<'EOF'
start=$PWD
( cd /tmp && echo "subshell in: $PWD" )
echo "back in start dir: $([[ $PWD == "$start" ]] && echo yes)"
{ echo "header"; echo "body"; } > report.txt; grep -c "" report.txt
SECONDS=0
for i in 1 2 3; do ( sleep 1; exit $(( i == 2 ? 7 : 0 )) ) & pids+=($!); done
for i in "${!pids[@]}"; do wait "${pids[$i]}"; echo "job $(( i + 1 )) exit=$?"; done
echo "3 jobs x 1s took ${SECONDS}s"
SECONDS=0
printf '%s\n' 1 2 3 4 | xargs -P 4 -I{} sleep 1
echo "xargs -P 4: ${SECONDS}s"
EOF
```

Expected output:

```text
subshell in: /tmp
back in start dir: yes
2
job 1 exit=0
job 2 exit=7
job 3 exit=0
3 jobs x 1s took 1s
xargs -P 4: 1s
```

Breakdown: a subshell is a separate process, and its `cd` disappears with it. Three one-second jobs took about 1 second because they ran in parallel. `wait PID` returns the code of that specific job — that's how you find out which parallel job failed (a bare `wait` would return 0). `xargs -P 4` gave the same effect for four commands. The time may show 1 or 2 seconds: `SECONDS` counts whole seconds.

</details>

## A robust script: strict mode, trap and a template

### `set -euo pipefail` — strict mode
By default Bash **keeps running after an error**. If `cd /app` failed, the following `rm -rf *` runs in the current directory. Strict mode changes that:

- **`set -e`** — exit the script if a command returned a non-zero code.
- **`set -u`** — an error when referring to an unset variable. It catches typos: `rm -rf "$BUILDDIR/"` with a typo in the name, without `-u`, turns into `rm -rf "/"`.
- **`set -o pipefail`** — a pipe's exit code is the code of the last *failed* command. Without it `curl ... | jq .` counts as successful even if curl failed, because jq successfully processed the empty input.
- **`set -E`** — the `ERR` trap (see below) fires inside functions too.

**Where `set -e` does NOT fire** (a frequent interview question):
```bash
set -e
if failing_cmd; then ...; fi        # in an if/while condition it's a test, not an error
failing_cmd || true                 # failure explicitly allowed
failing_cmd && echo ok              # in an && chain (except the last command)
local out=$(failing_cmd)            # the code is lost: local itself succeeds
```
The last case is sneaky. The right way is to separate the declaration and the assignment:
```bash
local out
out=$(failing_cmd)                  # now set -e will fire
```
And remember `(( x++ ))` with `x=0` (see the "Arithmetic" section).

### trap — actions on exit and on signals
`trap 'commands' SIGNAL` runs the commands when the script receives a signal or exits:
```bash
tmp_dir=$(mktemp -d)                         # a unique temporary folder
trap 'rm -rf "$tmp_dir"' EXIT                # EXIT — on ANY exit: success, error, exit
trap 'echo "interrupted by the user"; exit 130' INT TERM   # Ctrl+C and kill
trap 'echo "error on line $LINENO: $BASH_COMMAND" >&2' ERR   # when a command fails
```
Typical uses: deleting temporary files, releasing a lock, rolling back partially applied changes, sending a notification about the failure.

### Protection against running twice (cron)
If cron starts a backup every 5 minutes and the backup takes 7, the runs overlap. `flock` puts a lock on a file:
```bash
exec 9> /var/lock/backup.lock     # open the lock file on descriptor 9
if ! flock -n 9; then             # -n: don't wait — exit immediately if it's taken
  echo "already running, exiting" >&2
  exit 1
fi
# ... the main work. The lock is released by itself when the process exits
```

### Parsing options: getopts
```bash
usage() { echo "Usage: $0 [-n] [-d DAYS] NAMESPACE" >&2; exit 2; }

dry_run=false
days=7
while getopts ":nd:h" opt; do      # n — a flag; d: — an option WITH a value (a colon after d)
  case "$opt" in
    n) dry_run=true ;;
    d) days="$OPTARG" ;;           # the option's value is in OPTARG
    h) usage ;;
    :) echo "option -$OPTARG needs a value" >&2; usage ;;
    \?) echo "unknown option -$OPTARG" >&2; usage ;;
  esac
done
shift $(( OPTIND - 1 ))            # drop the parsed options; the positional arguments remain in $1
(( $# == 1 )) || usage
namespace="$1"
```
`getopts` is built into bash and understands only short options (`-n`, `-d 7`). For long ones (`--dry-run`) a loop is written by hand:
```bash
while (( $# > 0 )); do
  case "$1" in
    --dry-run) dry_run=true; shift ;;
    --days)    days="$2"; shift 2 ;;
    --days=*)  days="${1#*=}"; shift ;;
    -h|--help) usage ;;
    --)        shift; break ;;          # everything after -- is positional arguments
    -*)        echo "unknown option $1" >&2; usage ;;
    *)         break ;;
  esac
done
```

### A script template
Putting it all together. This template can serve as the basis for any automation script:
```bash
#!/usr/bin/env bash
# Deletes Evicted/Failed pods older than N days in the given namespace.
set -Eeuo pipefail

readonly SCRIPT_NAME="$(basename "$0")"

log()  { printf '%s [%s] %s\n' "$(date +%FT%T)" "$SCRIPT_NAME" "$*" >&2; }
die()  { log "ERROR: $*"; exit 1; }
usage() {
  cat >&2 <<EOF
Usage: $SCRIPT_NAME [-n] [-d DAYS] NAMESPACE
  -n        dry-run: only show what would be deleted
  -d DAYS   the minimum pod age in days (1 by default)
EOF
  exit 2
}

cleanup() { rm -rf "${TMP_DIR:-}"; }
trap cleanup EXIT
trap 'log "failed on line $LINENO: $BASH_COMMAND"' ERR

main() {
  local dry_run=false days=1 opt
  while getopts ":nd:h" opt; do
    case "$opt" in
      n) dry_run=true ;;
      d) days="$OPTARG" ;;
      *) usage ;;
    esac
  done
  shift $(( OPTIND - 1 ))
  (( $# == 1 )) || usage
  local ns="$1"

  command -v kubectl > /dev/null || die "kubectl not found"
  command -v jq > /dev/null || die "jq not found"
  TMP_DIR="$(mktemp -d)"

  local cutoff
  cutoff=$(date -u -d "-${days} days" +%s)        # GNU date; on macOS: gdate from coreutils

  kubectl get pods -n "$ns" --field-selector=status.phase=Failed -o json > "$TMP_DIR/pods.json"

  local name started deleted=0
  while IFS=$'\t' read -r name started; do
    if (( $(date -u -d "$started" +%s) < cutoff )); then
      if $dry_run; then
        log "would delete $name (started $started)"
      else
        kubectl delete pod -n "$ns" "$name" --wait=false
      fi
      (( ++deleted ))
    fi
  done < <(jq -r '.items[] | [.metadata.name, .status.startTime] | @tsv' "$TMP_DIR/pods.json")

  log "done: $deleted pods (dry_run=$dry_run)"
}

main "$@"
```
Why the code is in a `main` function: you can declare all the functions in any order, use `local`, and the script won't start running if the file was only partially downloaded (the last line `main "$@"` didn't arrive).

### 🧪 Mini task

**Task.** Build a script from the template: `set -Eeuo pipefail`, a temp directory removed in `trap … EXIT`, an error message in `trap … ERR`, `-n` and `-d DAYS` options via `getopts`, an argument count check. At the end — a pipe whose first command fails. Run it with valid options and with an unknown `-x`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > strict.sh <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"; echo "cleanup done" >&2' EXIT
trap 'echo "failed at line $LINENO: $BASH_COMMAND" >&2' ERR
usage() { echo "usage: $0 [-n] [-d DAYS] NAME" >&2; exit 2; }
dry_run=false; days=7
while getopts ":nd:" opt; do
  case "$opt" in
    n) dry_run=true ;;
    d) days=$OPTARG ;;
    *) usage ;;
  esac
done
shift $(( OPTIND - 1 ))
(( $# == 1 )) || usage
echo "name=$1 days=$days dry_run=$dry_run"
grep -c . /nope/file | sort
echo "never reached"
EOF
bash strict.sh -n -d 3 api; echo "exit=$?"
bash strict.sh -x; echo "exit=$?"
```

Expected output:

```text
name=api days=3 dry_run=true
grep: /nope/file: No such file or directory
failed at line 18: sort
cleanup done
exit=2
usage: strict.sh [-n] [-d DAYS] NAME
cleanup done
exit=2
```

Breakdown: options were parsed, then `grep` failed inside the pipe. Without `pipefail` the pipe would count as successful (the last command `sort` succeeded); instead the script stopped, the ERR trap showed the line and command, and `never reached` wasn't printed. `trap EXIT` removed the temp directory in both runs — on the error and on the `usage` exit. Exit code 2 for `usage` is the conventional "wrong usage" code.

</details>

## Text utilities used together with Bash
```bash
grep -rnE "ERROR|FATAL" --include="*.log" /var/log/app   # recursive, with line numbers, extended regex
grep -v "healthcheck" access.log         # lines NOT containing the pattern
grep -c "500" access.log                 # the number of matching lines
grep -A3 -B3 "Exception" app.log         # 3 lines of context after and before
grep -o 'user_id=[0-9]*' app.log         # print only the matched part

awk -F, '$3 > 100 { sum += $3 } END { print sum }' data.csv   # the sum of the 3rd CSV column where > 100
awk '$9 >= 500 { print $7 }' access.log  # request paths with 5xx (field 9 is the status, field 7 the path)

sed -n '10,20p' file                     # print lines 10–20
sed -i.bak 's/^#PermitRoot/PermitRoot/' /etc/ssh/sshd_config   # replace in the file, making a .bak backup
sed '/^$/d' file                         # delete empty lines

cut -d: -f1,7 /etc/passwd                # fields 1 and 7, with ':' as the separator
sort -t, -k2 -n data.csv                 # sort by the 2nd field numerically
sort -u                                  # sort and remove duplicates
tr 'a-z' 'A-Z'                           # replace characters
wc -l                                    # count lines
find . -type f -name "*.tmp" -mtime +7 -print0 | xargs -0 rm -f
# -print0 and -0: names are separated by a null byte — works with spaces and newlines in names

jq -r '.items[] | select(.status.phase != "Running") | .metadata.name' pods.json   # JSON
yq '.spec.replicas = 3' -i deploy.yaml   # YAML
envsubst < deploy.tpl.yaml > deploy.yaml # substitute environment variables into a template
```

### 🧪 Mini task

**Task.** From a five-line access.log find: the top 2 IPs by request count, the paths of 5xx requests with their statuses, the number of 5xx lines, the IP and status of all requests except health checks. From `pods.json` print the names of pods not in the `Running` phase with `jq`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > access.log <<'EOF'
10.0.0.5 - - [12/Oct/2026:10:00:00 +0000] "GET /api/health HTTP/1.1" 200 12
10.0.0.7 - - [12/Oct/2026:10:00:01 +0000] "POST /api/deploy HTTP/1.1" 503 0
10.0.0.5 - - [12/Oct/2026:10:00:02 +0000] "GET /api/pods HTTP/1.1" 200 512
10.0.0.9 - - [12/Oct/2026:10:00:03 +0000] "GET /api/pods HTTP/1.1" 502 0
10.0.0.5 - - [12/Oct/2026:10:00:04 +0000] "GET /healthz HTTP/1.1" 200 2
EOF
echo '{"items":[{"metadata":{"name":"api"},"status":{"phase":"Running"}},{"metadata":{"name":"etl"},"status":{"phase":"Failed"}}]}' > pods.json
echo "--- top IPs";   awk '{print $1}' access.log | sort | uniq -c | sort -rn | head -2
echo "--- 5xx paths"; awk '$9 >= 500 {print $9, $7}' access.log
echo "--- 5xx count"; grep -cE '" 5[0-9]{2} ' access.log
echo "--- no health"; grep -v health access.log | cut -d' ' -f1,9
echo "--- not running"; jq -r '.items[] | select(.status.phase != "Running") | .metadata.name' pods.json
```

Expected output:

```text
--- top IPs
   3 10.0.0.5
   1 10.0.0.9
--- 5xx paths
503 /api/deploy
502 /api/pods
--- 5xx count
2
--- no health
10.0.0.7 503
10.0.0.5 200
10.0.0.9 502
--- not running
etl
```

Breakdown: `sort | uniq -c | sort -rn` is the classic count; `uniq` only collapses adjacent lines, hence the first `sort`. awk splits fields on whitespace: `$9` is the status, `$7` the path (the date in the log takes two fields). `grep -c` with the `" 5..` pattern counts the status specifically, not any digit 5 in the line. `grep -v` excludes lines, `cut -d' ' -f1,9` takes the needed fields. `jq -r` prints strings without quotes.

</details>

## Debugging and quality

- **shellcheck** — a linter that finds almost all the traps from these notes: missing quotes, `read` without `-r`, `local x=$(cmd)`, `for f in $(ls)`. Every warning has a code (for example, SC2086) with an explanation page on the wiki. Mandatory in pre-commit and CI.
- **`bash -n script.sh`** — check the syntax without running it.
- **`bash -x script.sh`** or `set -x` inside the script — print every command *after* all expansions before executing it. You can immediately see what the variables turned into. To enable it for a section: `set -x; the problematic code; set +x`.
- **`PS4='+ ${BASH_SOURCE}:${LINENO}: '`** — add the file and line number to the trace.
- **shfmt** — auto-formatting (consistent indentation and style, like `terraform fmt`), **bats-core** — tests for bash scripts (`.bats` files with checks like "run the script and make sure the exit code is 0 and the output contains X").

### 🧪 Mini task

**Task.** Run a script with typical mistakes (`for f in $(ls $dir)`, unquoted variables) through `shellcheck`. Check the syntax of an unclosed `if` with `bash -n`. Look at a `bash -x` trace with line numbers via `PS4`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > bad.sh <<'EOF'
#!/usr/bin/env bash
dir=$1
for f in $(ls $dir); do
  echo $f
done
EOF
shellcheck -f gcc bad.sh
printf 'if true; then\n  echo ok\n' > broken.sh
bash -n broken.sh 2>&1 | sed 's/^.*broken.sh: //'
PS4='+ line ${LINENO}: ' bash -x -c 'name=web; port=$((8000 + 80)); echo "$name:$port"'
```

Expected output:

```text
bad.sh:3:10: error: Iterating over ls output is fragile. Use globs. [SC2045]
bad.sh:3:15: note: Double quote to prevent globbing and word splitting. [SC2086]
bad.sh:4:8: note: Double quote to prevent globbing and word splitting. [SC2086]
line 3: syntax error: unexpected end of file from `if' command on line 1
+ line 1: name=web
+ line 1: port=8080
+ line 1: echo web:8080
web:8080
```

Breakdown: shellcheck found three problems and gave each a code. SC2045 — iterating over `ls` output breaks on spaces in names (correct: `for f in "$dir"/*`), SC2086 — unquoted variables. You can open the explanation by code: shellcheck.net/wiki/SC2086. `bash -n` found the unclosed `if` without running anything — handy in pre-commit. The `+ line 1:` trace shows commands *after* expansion: `port=8080` is already computed.

</details>

## Useful mini-scripts

**Checking SSL certificate expiry:**
```bash
#!/usr/bin/env bash
set -euo pipefail
for domain in example.com api.example.com; do
  # s_client connects to the server; x509 -enddate extracts the expiry date from the certificate
  end_date=$(echo | openssl s_client -servername "$domain" -connect "$domain:443" 2> /dev/null \
             | openssl x509 -noout -enddate | cut -d= -f2)
  days_left=$(( ( $(date -d "$end_date" +%s) - $(date +%s) ) / 86400 ))
  printf '%-25s %4d days\n' "$domain" "$days_left"
  (( days_left < 14 )) && echo "  WARNING: expires soon" >&2
done
```

**A database backup with rotation (keep the last 7):**
```bash
#!/usr/bin/env bash
set -euo pipefail
backup_dir=/backup
file="$backup_dir/db-$(date +%F_%H%M).sql.gz"

pg_dump "$DATABASE_URL" | gzip > "$file"   # pipefail: if pg_dump fails, the whole pipe fails
echo "created $file ($(du -h "$file" | cut -f1))"

# ls -1t — by time, newest first; tail -n +8 — everything from the 8th on; xargs -r — don't run rm if the list is empty
ls -1t "$backup_dir"/db-*.sql.gz | tail -n +8 | xargs -r rm --
```

**Waiting for a dependency to come up (a container entrypoint):**
```bash
#!/usr/bin/env bash
set -euo pipefail
host="${DB_HOST:?}"; port="${DB_PORT:-5432}"
for i in {1..30}; do
  nc -z "$host" "$port" && break
  echo "waiting for $host:$port ($i/30)" >&2
  sleep 2
done
nc -z "$host" "$port" || { echo "the database didn't come up" >&2; exit 1; }
exec "$@"     # replace the shell with the application process: it becomes PID 1 and receives signals
```

### 🧪 Mini task

**Task.** Make `wait-for.sh` — an entrypoint that checks a port with `nc -z` up to 3 times, fails with a clear error if the port never opens, and on success starts the given command via `exec`. Test it on a closed port and on the port of a local `python3 -m http.server`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > wait-for.sh <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
host="${HOST:?}"; port="${PORT:?}"
for i in 1 2 3; do
  nc -z "$host" "$port" 2>/dev/null && break
  echo "waiting for $host:$port ($i/3)" >&2
  sleep 1
done
nc -z "$host" "$port" 2>/dev/null || { echo "$host:$port is down" >&2; exit 1; }
exec "$@"
EOF
python3 -m http.server 8766 --bind 127.0.0.1 >/dev/null 2>&1 &
server=$!
sleep 1
HOST=127.0.0.1 PORT=8799 bash wait-for.sh echo "app started"; echo "exit=$?"
HOST=127.0.0.1 PORT=8766 bash wait-for.sh echo "app started"; echo "exit=$?"
kill $server
```

Expected output:

```text
waiting for 127.0.0.1:8799 (1/3)
waiting for 127.0.0.1:8799 (2/3)
waiting for 127.0.0.1:8799 (3/3)
127.0.0.1:8799 is down
exit=1
app started
exit=0
```

Breakdown: on the closed port there were three attempts, a message to stderr and exit code 1 — the container fails and the orchestrator restarts it. On the open port the check passed at once, and `exec` replaced the script process with the `echo` command: that's how an app in a container becomes PID 1 and receives `SIGTERM` itself. `${HOST:?}` ensures the script won't run "blind" without the variable.

</details>

## Live coding: text-processing tasks

DevOps/SRE interviews often include a 10–20 minute mini task in a shared editor (CoderPad, HackerRank) or in a terminal over screen sharing: "here are a few lines — print the third word of each line on one line", "count the statuses in access.log", "which hosts from the list are not updated yet". The goal is not knowledge of rare flags but the ability to build a pipeline from simple utilities and to think about edge cases. The same tasks solved in Python are in the Python topic, section "Live coding: the same tasks in Python".

**How to solve out loud:**
1. **Clarify the input.** What separates the fields: space, tab, comma? Can there be empty lines or lines shorter than needed? Does case matter? In what order should the output be?
2. **Build the pipeline step by step.** Start with the first command, look at the intermediate result, then add `| the next one`. The interviewer sees your reasoning, and you spot mistakes immediately.
3. **Check edge cases:** an empty line, extra spaces, a file without a trailing newline, an empty file.
4. **Name an alternative:** "in Python I would use `Counter`", "on big files `awk` is faster than a `while read` loop".

**Setup.** All the tasks below use the same files. Paste the whole block into a terminal — it creates a temporary directory with the samples and switches to it:

```bash
cd "$(mktemp -d)"
cat > lines.txt <<'EOF'
the quick brown fox
jumps over the lazy dog
a b c
only two
EOF
cat > words.txt <<'EOF'
Deploy api, deploy web.
Rollback; deploy API!
restart db
EOF
cat > access.log <<'EOF'
10.0.0.1 - - [26/Oct/2026:10:00:01 +0000] "GET / HTTP/1.1" 200 512 "-" "curl/8.0"
10.0.0.2 - - [26/Oct/2026:10:00:02 +0000] "GET /api HTTP/1.1" 500 128 "-" "curl/8.0"
10.0.0.1 - - [26/Oct/2026:10:00:03 +0000] "POST /login HTTP/1.1" 302 0 "-" "Mozilla/5.0"
10.0.0.3 - - [26/Oct/2026:10:00:04 +0000] "GET /api HTTP/1.1" 200 256 "-" "curl/8.0"
10.0.0.1 - - [26/Oct/2026:10:00:05 +0000] "GET /missing HTTP/1.1" 404 64 "-" "curl/8.0"
10.0.0.2 - - [26/Oct/2026:10:00:06 +0000] "GET /api HTTP/1.1" 502 0 "-" "curl/8.0"
EOF
printf '%s\n' web api web db api > services.txt
printf '%s\n' web-1 web-2 db-1 cache-1 api-1 > all.txt
printf '%s\n' db-1 web-1 > done.txt
cat > times.txt <<'EOF'
GET /api 200 0.120
GET /web 200 0.080
GET /api 500 1.500
GET /db 200 0.300
EOF
cat > users.csv <<'EOF'
name,email,age
alice,alice@example.com,34
bob,bob@example.com,27
carol,carol@example.com,41
EOF
mkdir -p data/logs
head -c 3000000 /dev/zero > data/logs/app.log
head -c 2000000 /dev/zero > data/logs/old.log
head -c 1000000 /dev/zero > data/dump.sql
head -c 10000 /dev/zero > data/readme.txt
cat > app.conf <<'EOF'
host=0.0.0.0
port=80
debug=true
EOF
```

### 1. The third word of each line — on one line

**Task.** `lines.txt` has several lines. Print the third word of each line, all on one line separated by spaces. Skip lines with fewer than three words.

```text
the quick brown fox
jumps over the lazy dog
a b c
only two
```

Expected output: `brown the c`.

<details class="qa"><summary>Solution and walkthrough</summary>

```bash
awk 'NF >= 3 {print $3}' lines.txt | paste -sd ' ' -
```

```text
brown the c
```

How it works:
- `awk` reads the file line by line and splits each line into fields on spaces and tabs by itself; several spaces in a row count as one separator. The fields are `$1`, `$2`, `$3`, and `NF` (number of fields) is how many fields the current line has.
- `NF >= 3 {print $3}` is a "condition {action}" pair: print the third field only where there are at least three fields. The line `only two` is skipped.
- `paste -sd ' ' -` joins the lines into one: `-s` — all input lines in a row (serial), `-d ' '` — the separator is a space, `-` — read from stdin. On macOS the command does not work without `-`; on Linux it is allowed too — always write it.

Other ways to put the result on one line — worth naming at least one:

```bash
awk 'NF >= 3 {print $3}' lines.txt | xargs              # xargs without a command prints all arguments via echo
awk 'NF >= 3 {print $3}' lines.txt | tr '\n' ' '; echo  # newlines -> spaces, but a trailing space remains
awk 'NF >= 3 {printf "%s%s", sep, $3; sep = " "} END {print ""}' lines.txt   # all in one awk
```

In the last variant the variable `sep` is empty on the first line (all awk variables are empty by default) and becomes a space after the first output. So there is no extra space at the start or at the end. `END {print ""}` runs after the last line and adds the final newline.

The same in pure Bash, without external utilities:

```bash
while read -r _ _ third _; do
  [[ -n $third ]] && printf '%s ' "$third"
done < lines.txt; echo
```

`read` distributes the words of a line across variables: the first goes to `_`, the second to `_`, the third to `third`, everything else to the last `_` (the name `_` means "not needed"). If there are fewer than three words, `third` is empty and the `[[ -n $third ]]` check skips it.

**Common mistakes:**
- **`cut -d' ' -f3`** treats every single space as a separator. On the line `"  a   b  c"` the third field is `a`, although the third word is `c`. `awk` parses such lines correctly.
- **`awk '{print $3}'` without a condition** prints an empty line for `only two`, and `paste` leaves a trailing space: `brown the c `.
- **`xargs` on "dirty" data** breaks on quotes and backslashes — for arbitrary text `paste` is more reliable.

Likely follow-ups: "what if the separator is a comma?" → `awk -F, '{print $3}'`; "the last word?" → `$NF`; "the second to last?" → `$(NF-1)`.

</details>

### 2. The two most frequent words

**Task.** Count which words occur most often in `words.txt`. Ignore case and punctuation: `Deploy`, `deploy` and `API!` are `deploy` and `api`. Print the two most frequent with their counts.

```text
Deploy api, deploy web.
Rollback; deploy API!
restart db
```

<details class="qa"><summary>Solution and walkthrough</summary>

```bash
tr -cs '[:alpha:]' '\n' < words.txt | tr '[:upper:]' '[:lower:]' | sort | uniq -c | sort -rn | head -2
```

```text
   3 deploy
   2 api
```

The pipeline step by step — this is exactly how to build it in an interview, checking the output after each `|`:
1. `tr -cs '[:alpha:]' '\n'` — `-c` (complement) takes every character **except** letters, `-s` (squeeze) collapses a run of such characters into one. Result: every run of spaces and punctuation becomes one newline — one word per line.
2. `tr '[:upper:]' '[:lower:]'` — everything to lower case, so `Deploy` and `deploy` count as one word.
3. `sort` — puts identical words next to each other. This is required: the next step counts only **adjacent** repeats.
4. `uniq -c` — collapses adjacent identical lines and writes the count in front of each.
5. `sort -rn` — sort by number (`-n`) in reverse order (`-r`): the most frequent on top.
6. `head -2` — the first two lines.

**Common mistakes:**
- **`uniq` without `sort`** — repeats that are not adjacent are not merged, and the count is wrong.
- **`sort -r` without `-n`** compares as text: `9` ends up above `10` because the character `9` is greater than the character `1`.
- **Forgetting to normalise case** — `Deploy` and `deploy` are counted separately.
- **The text starts with punctuation** — the first line after `tr` is empty. It gets counted as an "empty word"; remove it with `grep -v '^$'`.

Likely follow-ups:
- "If several words have the same count — how do you order them alphabetically?" → `sort -k1,1nr -k2,2`: first by the first field as a number in reverse order, on a tie by the second field as text.
- "How do you count without `sort | uniq`?" → an awk array: `awk '{c[$0]++} END {for (w in c) print c[w], w}'`. It keeps only the unique words in memory, while `sort` on a huge file writes temporary files to disk.

</details>

### 3. How many requests per HTTP status

**Task.** You have an `access.log` in the standard nginx (combined) format. Count how many requests there were for each HTTP status, and the share of 5xx responses.

<details class="qa"><summary>Solution and walkthrough</summary>

First break one line into fields — awk splits on spaces:

```text
10.0.0.2 - - [26/Oct/2026:10:00:02 +0000] "GET /api HTTP/1.1" 500 128 "-" "curl/8.0"
   $1   $2 $3         $4             $5    $6   $7     $8     $9  $10 $11    $12
```

The status is the ninth field:

```bash
awk '{print $9}' access.log | sort | uniq -c | sort -rn
```

```text
   2 200
   1 502
   1 500
   1 404
   1 302
```

The 5xx share — with a single awk:

```bash
awk '$9 ~ /^5/ {e++} END {printf "5xx: %d of %d (%.1f%%)\n", e, NR, 100*e/NR}' access.log
```

```text
5xx: 2 of 6 (33.3%)
```

- `$9 ~ /^5/` is the condition "field 9 matches the regular expression `^5`", that is, it starts with a five. For such lines we increment the counter `e`.
- `NR` (number of records) in the `END` block is the total number of lines read.
- `%%` in `printf` is a percent sign; a single `%` starts a format.

**Common mistakes:**
- **A broken request line.** For a garbage request nginx writes `"-"` instead of `"GET / HTTP/1.1"`, the fields shift, and `$9` holds `"-"` instead of the status. It is more reliable to split on quotes: the status always comes right after the second quote.

  ```bash
  awk -F'"' '{split($3, a, " "); print a[1]}' access.log | sort | uniq -c | sort -rn
  ```

  With `-F'"'` the field `$3` is the piece between the second and third quotes: ` 500 128 `. `split` cuts it on spaces into the array `a`, and `a[1]` is the status.
- **The order of lines with the same count** (`502`, `500`, `404`, `302`) is not guaranteed — if it matters, add a second sort key: `sort -k1,1nr -k2,2`.

Likely follow-ups: "top IPs" → `awk '{print $1}' access.log | sort | uniq -c | sort -rn | head`; "which URLs return 5xx" → `awk '$9 >= 500 {print $7}' access.log | sort | uniq -c`.

</details>

### 4. Remove duplicates while keeping the order

**Task.** `services.txt` has duplicates. Print each line once, **in the order of first appearance**.

```text
web
api
web
db
api
```

Expected output: `web`, `api`, `db`.

<details class="qa"><summary>Solution and walkthrough</summary>

```bash
awk '!seen[$0]++' services.txt
```

```text
web
api
db
```

A classic one-liner idiom worth understanding piece by piece:
- `seen` is an associative array (a dictionary), `$0` is the whole current line.
- `seen[$0]++` returns the **old** value and only then increments it. The first time for the line `web` the old value is empty (that is, 0), the second time it is 1.
- `!` inverts it: the condition is true only on the first encounter of a line.
- A condition without an `{action}` in awk means "print the line".

Why the obvious options do not fit:

```bash
sort -u services.txt    # api db web — duplicates removed, but the order became alphabetical
uniq services.txt       # web api web db api — uniq removes only adjacent duplicates
```

Likely follow-ups: "show only the duplicated ones" → `sort services.txt | uniq -d` (prints `api` and `web`); "how many times each" → `sort | uniq -c`.

</details>

### 5. What is in one list but not in the other

**Task.** `all.txt` has all hosts, `done.txt` has the ones already updated. Print the hosts that are not updated yet.

```text
all.txt: web-1 web-2 db-1 cache-1 api-1      done.txt: db-1 web-1
```

<details class="qa"><summary>Solution and walkthrough</summary>

```bash
grep -Fxvf done.txt all.txt
```

```text
web-2
cache-1
api-1
```

The flags one by one:
- `-f done.txt` — take the search patterns from a file, one per line.
- `-F` — the patterns are plain strings, not regular expressions (the dot in `db.local` will not mean "any character").
- `-x` — the pattern must match the **whole** line, not a part of it. Without it `web-1` is also found inside `web-10`.
- `-v` — invert: print the lines that did **not** match.

The order is kept as in `all.txt`. The second classic way is `comm`, which needs sorted inputs:

```bash
comm -23 <(sort all.txt) <(sort done.txt)
```

```text
api-1
cache-1
web-2
```

`comm` prints three columns: lines only in the first file, only in the second, and in both. `-23` hides the second and third — what remains is "only in the first". `<(sort all.txt)` is process substitution: Bash passes the command's output as if it were a file.

**Common mistakes:**
- **Without `-x`** an empty line in `done.txt` (for example, an extra trailing newline) matches any line — and `grep -Fvf` prints **nothing**. With `-x` an empty pattern matches only an empty line.
- **A file from Windows** has an invisible `\r` at the end of lines, and `db-1\r` is not equal to `db-1`. Fix it with `tr -d '\r' < done.txt`.
- **`comm` on unsorted files** produces garbage without any error.

</details>

### 6. Average and maximum response time

**Task.** In `times.txt` the fourth field is the response time in seconds. Print the number of requests, the average time and the slowest request with its path.

```text
GET /api 200 0.120
GET /web 200 0.080
GET /api 500 1.500
GET /db 200 0.300
```

<details class="qa"><summary>Solution and walkthrough</summary>

```bash
awk '
  { sum += $4 }
  NR == 1 || $4 > max { max = $4; path = $2 }
  END { if (NR) printf "requests=%d avg=%.3f max=%.3f (%s)\n", NR, sum / NR, max, path }
' times.txt
```

```text
requests=4 avg=0.500 max=1.500 (/api)
```

- An awk program is a set of `condition { action }` blocks, each applied to every line. A block without a condition (`{ sum += $4 }`) runs for all lines.
- `NR == 1 || $4 > max` — on the first line we remember it as the maximum, and after that we update only when we find a larger value. Without `NR == 1` the comparison with an empty `max` works while all values are positive but breaks on negative numbers — it is better to initialise explicitly.
- `if (NR)` in `END` protects against division by zero on an empty file.

**Why not Bash arithmetic:** `$(( ))` works only with integers — `$(( 0.12 + 0.08 ))` gives a syntax error. Fractional numbers are computed in `awk` or `bc`.

Likely follow-up: "what about p95?" — sort the times (`sort -n -k4,4`) and take the value at the position of 95% of the line count. On four values this is meaningless, on a real log it is a fine answer.

</details>

### 7. Filtering a CSV

**Task.** `users.csv` has the columns `name,email,age`. Print the name and email of everyone older than 30.

<details class="qa"><summary>Solution and walkthrough</summary>

```bash
awk -F, 'NR > 1 && $3 > 30 {print $1, $2}' users.csv
```

```text
alice alice@example.com
carol carol@example.com
```

- `-F,` — the field separator is a comma instead of a space.
- `NR > 1` — skip the first line with the header.
- `$3 > 30` — awk itself understands that `34` is a number and compares numerically.
- `print $1, $2` — the comma in `print` inserts the output separator (a space by default). To make the output CSV too: `awk -F, 'BEGIN {OFS = ","} ...'`.

**The main trap:** real CSV can contain a comma inside a quoted value: `"Smith, John",j@example.com,45`. awk will split such a value into two fields. For real CSV use the Python `csv` module (see the Python topic) or the `mlr`/`csvkit` utilities — and say so in the interview, it counts in your favour.

</details>

### 8. Reverse a string and the word order

**Task.** For the string `hello world` print it backwards (`dlrow olleh`) and the words in reverse order (`world hello`).

<details class="qa"><summary>Solution and walkthrough</summary>

```bash
echo "hello world" | rev
echo "hello world" | awk '{for (i = NF; i > 0; i--) printf "%s%s", $i, (i > 1 ? " " : "\n")}'
```

```text
dlrow olleh
world hello
```

- `rev` reverses the characters in each line.
- In awk the loop goes from the last field (`NF`) to the first. `(i > 1 ? " " : "\n")` is the ternary operator: after every word except the last one printed we put a space, at the end a newline.

If asked to do it without external utilities — in pure Bash:

```bash
s="hello world"; out=""
for (( i = ${#s} - 1; i >= 0; i-- )); do out+="${s:i:1}"; done
echo "$out"
```

`${#s}` is the length of the string, `${s:i:1}` is a one-character substring starting at position `i`. The loop goes from the end and appends characters to `out`.

To reverse the order of the **lines** of a file: `tac file` on Linux, `tail -r file` on macOS.

</details>

### 9. The largest files in a directory

**Task.** Find the three largest files inside `data/` (including subdirectories).

<details class="qa"><summary>Solution and walkthrough</summary>

```bash
find data -type f -exec du -k {} + | sort -rn | head -3
```

```text
2932	data/logs/app.log
1956	data/logs/old.log
980	data/dump.sql
```

- `find data -type f` — all regular files recursively, without directories.
- `-exec du -k {} +` — run `du` on the files found; `+` passes them in batches to a single call instead of running `du` for each file. `-k` — size in kilobytes.
- `sort -rn | head -3` — descending by number, the first three.

Variants worth knowing:
- `find data -type f -printf '%s %p\n' | sort -rn | head -3` — the exact size in bytes; `-printf` exists only in GNU find (Linux).
- `du -ah data | sort -rh | head -3` — human-friendly sizes like `2.9M`, but `du -a` also prints **directories**: `data` itself (5.7M) comes first, then `data/logs`. This is a common mistake.
- `find data -type f -size +1M` — files larger than 1 mebibyte. Trap: `dump.sql` with 1,000,000 bytes does **not** get in, because `1M` is 1,048,576 bytes.

Likely follow-up: "the disk is full, but `du` shows little used space" → a deleted file still held open by a process, found with `lsof +L1` (details in the Linux topic, the file system section).

</details>

### 10. Change values in a config

**Task.** In `app.conf` set `port=8080` and `debug=false`, and if there is no `timeout` key — append `timeout=30`. Running it again must not break anything.

```text
host=0.0.0.0
port=80
debug=true
```

<details class="qa"><summary>Solution and walkthrough</summary>

```bash
sed -i.bak -e 's/^port=.*/port=8080/' -e 's/^debug=.*/debug=false/' app.conf
grep -q '^timeout=' app.conf || echo 'timeout=30' >> app.conf
cat app.conf
```

```text
host=0.0.0.0
port=8080
debug=false
timeout=30
```

- `s/^port=.*/port=8080/` — a substitution: find the line that **starts** (`^`) with `port=` and replace it entirely (`.*` — everything up to the end of the line).
- `-e` lets you pass several expressions in one `sed` call.
- `-i.bak` — edit the file in place and keep a copy `app.conf.bak`. Exactly this form works in both GNU sed (Linux) and BSD sed (macOS); `sed -i` without a suffix on macOS requires an empty argument `sed -i ''`.
- `grep -q` prints nothing and only returns a code: 0 — found, 1 — not found. `||` runs `echo` only on a non-zero code — the key is appended once. So the script is **idempotent**: a second run changes nothing.

**Common mistakes:**
- **Without `^`** the expression `s/port=.*/.../` also hits `export=...` and `proxy_port=...`.
- **A slash in the value** (`path=/var/log`) breaks `s/.../.../`. Use a different delimiter: `sed 's|^path=.*|path=/var/log|'`.
- **`echo ... > app.conf`** instead of `>>` overwrites the whole file.

In real work such edits are done with templates or Ansible (`lineinfile`, `template`), but in an interview `sed` + `grep -q ||` is the expected answer.

</details>

### 11. FizzBuzz — a warm-up

**Task.** Print the numbers from 1 to 15 on one line, but instead of multiples of 3 write `Fizz`, multiples of 5 — `Buzz`, multiples of both 3 and 5 — `FizzBuzz`.

<details class="qa"><summary>Solution and walkthrough</summary>

```bash
for i in {1..15}; do
  if (( i % 15 == 0 )); then echo FizzBuzz
  elif (( i % 3 == 0 )); then echo Fizz
  elif (( i % 5 == 0 )); then echo Buzz
  else echo "$i"
  fi
done | paste -sd ' ' -
```

```text
1 2 Fizz 4 Buzz Fizz 7 8 Fizz Buzz 11 Fizz 13 14 FizzBuzz
```

- `(( ))` is an arithmetic context, `%` is the remainder of division. `i % 3 == 0` means "divisible by 3 without a remainder".
- The output of the whole loop can be sent into a pipeline: `done | paste ...` joins the lines into one.

**Common mistakes:**
- **The check for 15 is not first** — then the `Fizz` branch fires for 15, and `FizzBuzz` is never reached.
- **`{1..$n}` with a variable does not work**: brace expansion happens before variable substitution, and `echo {1..$n}` prints literally `{1..3}`. For a variable bound: `for (( i = 1; i <= n; i++ ))` or `seq 1 "$n"`.

</details>

## Interview questions

**What does `set -euo pipefail` do? What are its pitfalls?**
`-e` exits the script when a command fails, `-u` — when an unset variable is referenced, `pipefail` makes a pipe fail if any of its parts failed. The pitfalls: `-e` doesn't fire in `if`/`while` conditions, in `cmd || ...`, `cmd && ...`, in `local x=$(cmd)` (the code is lost because of `local`); `(( x++ ))` with `x=0` unexpectedly exits the script.

**Why must variables be quoted?**
Without quotes the value goes through word splitting and filename expansion. A file name with a space turns into several arguments, `*` expands into a list of files, an empty variable disappears from the command entirely. `rm -rf $DIR/` with an empty `DIR` becomes `rm -rf /`.

**How do `"$@"`, `"$*"` and `$@` differ?**
`"$@"` passes each argument as a separate word, preserving the spaces inside. `"$*"` joins all the arguments into one string. Unquoted `$@` splits arguments with spaces into pieces. Wrappers almost always need `"$@"`.

**How do `[ ]`, `[[ ]]` and `(( ))` differ?**
`[` is the external POSIX `test` command: its arguments go through word splitting, so quotes are mandatory. `[[` is a built-in bash construct: no word splitting, with `&&`, `||`, patterns and `=~`. `((` is arithmetic and number comparison.

**Why are `for f in $(ls)` and `cat file | while read line` bad?**
`$(ls)` splits file names with spaces — use a glob, `for f in *.log`. A `while read` in a pipe runs in a subshell, and variables changed in the loop aren't visible after it — use `while ...; done < file` or `< <(cmd)`. Plus `read` without `-r` mangles backslashes.

**How does a function return a value?**
`return` sets only the exit code 0–255 (success or error). A function prints data to stdout, and the caller captures it with `$(func)`. That's why logs inside a function go to stderr. An alternative is writing into a variable by name via `local -n`.

**What is a subshell and when is it created?**
A child process that's a copy of the shell. It's created for `( ... )`, `$( ... )`, every part of a pipe and background jobs with `&`. Changes to variables, `cd` and options in it aren't visible to the parent.

**What does `2>&1` mean and why does the order matter?**
"Send stderr to where stdout is currently going". `cmd > file 2>&1` writes both streams to the file. `cmd 2>&1 > file` sends stderr to the terminal (where stdout pointed before the redirection), and only stdout ends up in the file.

**How does `source script.sh` differ from `./script.sh`?**
`source` executes the script in the current shell: its variables, functions and `cd` remain. `./script.sh` runs in a new process, and after it finishes nothing is left of it except the exit code.

**Why is `export` needed?**
Without it a variable is visible only to the current shell. With `export` it goes into the environment and is visible to the programs that are started. The form `VAR=value cmd` sets a variable for one command only.

**How do you guarantee that temporary files are deleted on exit?**
`tmp=$(mktemp -d)` and `trap 'rm -rf "$tmp"' EXIT`. The EXIT trap fires on any termination of the script: normal, on an error with `set -e`, via `exit`.

**How do you prevent a cron script from running twice?**
`flock`: `exec 9> /var/lock/name.lock; flock -n 9 || exit 1`. The lock is released automatically when the process exits.

**How do you parse a script's arguments?**
`getopts` for short options (built-in, POSIX). For long options — a `while (( $# )); do case "$1" in ... esac; done` loop with `shift`.

**What's the trap in `a && b || c`?**
`c` runs if `a` **or** `b` fails. It's not if/else. Branching needs an `if`.

**How do you debug a bash script?**
`bash -x` (or `set -x`) — tracing commands after expansions; `PS4` with `$LINENO` for line numbers; `trap '...' ERR`; `bash -n` for a syntax check; `shellcheck` for finding typical mistakes.

**Why `exec "$@"` at the end of a container's entrypoint script?**
`exec` replaces the shell process with the application process. The application becomes PID 1 and receives signals directly (SIGTERM from `docker stop` or Kubernetes). Without `exec` PID 1 is bash, which doesn't pass SIGTERM on to the child process, and the container is killed by SIGKILL after the timeout without a graceful shutdown.

**When should a bash script be rewritten in Python?**
When you need data structures, work with JSON or APIs, complex error handling, tests, or the script has grown beyond 100–200 lines.

**A task: a script checks the availability of hosts from a file and records the unreachable ones.**
```bash
#!/usr/bin/env bash
set -euo pipefail
input="${1:?Usage: $0 hosts.txt}"
output="down.txt"
: > "$output"                          # ':' + a redirection = create or truncate the file

while IFS= read -r host || [[ -n $host ]]; do
  [[ -z $host || $host == \#* ]] && continue           # skip empty lines and comments
  if ! ping -c1 -W2 "$host" &> /dev/null; then
    echo "$host" >> "$output"
  fi
done < "$input"

echo "Unreachable: $(wc -l < "$output") of $(grep -cv '^\s*\(#\|$\)' "$input")"
```
What's worth talking through: why `IFS=` and `-r`, why `|| [[ -n $host ]]`, why `wc -l < file` (no file name in the output), that `ping -W` means different things on Linux and macOS (seconds and milliseconds), and how to parallelise the check (`xargs -P` or background jobs with `wait`).
