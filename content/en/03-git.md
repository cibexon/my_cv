# Git

Git in depth: how it works inside (this explains the behaviour of every command), day-to-day work, merge and rebase, analysing heavily branched repositories, moving changes between branches, rescuing lost work and branching strategies. At the end — cases that come up in interviews.

## How Git works inside

Many Git commands seem like magic until you understand the data model. It's simple.

### Snapshots, not diffs
Git doesn't store "file changes" — it stores a **snapshot of the whole project** at the moment of each commit. Unchanged files are not duplicated: the new snapshot points to the same object as the previous one. Git computes diffs on the fly when you ask it to show changes.

### Four object types
Everything is stored in the `.git/objects` folder as objects, and each object's name is the **hash of its content** (SHA-1 — an algorithm that computes a fixed-length "fingerprint" from any data, 40 hexadecimal characters; change a single byte and the whole hash changes):
- **blob** — the content of one file (without its name);
- **tree** — the content of a directory: a list of "name → blob or nested tree";
- **commit** — a reference to the root tree (the project snapshot), references to the **parent** commits, author, date, message;
- **tag** (annotated) — a named reference to a commit with an author and a message.

```bash
git cat-file -p HEAD
# tree 9c4a1f...           ← the project snapshot
# parent 3b7e2d...         ← the previous commit
# author Alex <alex@...> 1760000000 +0300
# committer Alex <...>
#
# Add healthcheck endpoint
git cat-file -p 9c4a1f     # the tree's content: which files and folders
```

The main consequences:
1. **A commit is immutable.** The hash is computed from the content, including the parent's hash. Changing the message, a file or the parent means getting a **new** commit with a new hash. That is why `--amend`, `rebase` and `cherry-pick` don't change commits — they create new ones.
2. **History is a chain of references to parents.** A commit knows its parents but not its descendants. A merge commit has two parents.
3. **Integrity.** You can't quietly swap an old commit: its hash and the hashes of all its descendants would change.

### Branches and HEAD are just pointers
A **branch** is a file containing the hash of one commit (`.git/refs/heads/main`). Nothing more. Creating a branch means writing 41 bytes, which is why branches are cheap in Git.

When you commit on a branch, the new commit gets the current one as its parent, and the branch pointer **moves** to the new commit.

**HEAD** is a pointer to where you are now. It usually points to a branch (`ref: refs/heads/main`), and the branch points to a commit.

```
            HEAD
             ↓
            main
             ↓
A ← B ← C ← D          (each commit points to its parent)
         ↑
       feature
```

**Detached HEAD** — HEAD points directly to a commit rather than to a branch (after `git checkout <hash>` or checking out a tag). New commits made in this state won't end up on any branch, and after switching away you'll only find them through the reflog. To keep the work — create a branch: `git switch -c rescue`.

**Remote branches** (`origin/main`) are local copies of the server's pointers as of the last `fetch`. They don't update by themselves.

### Three areas: working copy, index, repository
```
Working copy           Index (staging area)         Repository (.git)
(files on disk)   --git add-->  (what goes into     --git commit-->  (commits)
                                the next commit)
```
The **index** is an intermediate area where the next commit is assembled. Why it exists: you can commit only part of your changes (for example, a bug fix separately from a refactoring), even parts of one file (`git add -p`).

```bash
git status            # what is changed in the working copy, what is added to the index
git diff              # working copy vs index (what is NOT yet added)
git diff --staged     # index vs the last commit (what WILL go into the commit)
```

### 🧪 Mini task

**Task.** Look inside a repository: compute the hash of a file's content before committing, commit two files with identical text, and see which objects appeared, what the commit's tree contains and what `HEAD` really is.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo hello > hello.txt
git hash-object hello.txt
cp hello.txt copy.txt
git add . && git commit -qm "two files"
git cat-file -t HEAD
git cat-file -p 'HEAD^{tree}'
git cat-file -t ce013625030ba8dba906f756967f9e9ca394464a
cat .git/HEAD
```

Expected output:

```text
ce013625030ba8dba906f756967f9e9ca394464a
commit
100644 blob ce013625030ba8dba906f756967f9e9ca394464a	copy.txt
100644 blob ce013625030ba8dba906f756967f9e9ca394464a	hello.txt
blob
ref: refs/heads/main
```

Breakdown: a blob's hash depends only on the content (`hello` plus a newline), so it's the same on any machine and for both files — Git stores that text once. File names live not in the blob but in the tree. A commit points to a tree, and `HEAD` is just a text file referring to the current branch. The branch itself is the file `.git/refs/heads/main` holding a commit hash. Commit hashes differ for everyone: they include the author and the time.

</details>

## Day-to-day work

### Setup
```bash
git config --global user.name "Alex Sukhanov"
git config --global user.email "alex@example.com"
git config --global pull.rebase true          # pull rebases instead of merging (linear history)
git config --global rerere.enabled true       # remember conflict resolutions (see below)
git config --global init.defaultBranch main
git config --global alias.lg "log --oneline --graph --decorate --all"
```

### A typical cycle of working on a task
```bash
git switch main && git pull                  # a fresh main
git switch -c feature/JIRA-123-healthcheck   # a new branch from the current commit

# ... edits ...
git status -sb                               # short status and how far behind the remote branch you are
git add -p                                   # add changes hunk by hunk, reviewing each one
git commit -m "feat: add /health endpoint"

git push -u origin feature/JIRA-123-healthcheck   # -u: remember the link to the remote branch
# from then on simply git push / git pull
```
`git switch` and `git restore` (Git 2.23+) are a clearer replacement for the overloaded `git checkout`, which could both switch branches and revert files.

### Undoing changes before a commit
```bash
git restore app.py              # discard changes in a file (restore it as in the index) — IRREVERSIBLE
git restore --staged app.py     # remove a file from the index (the changes in the working copy stay)
git restore --source=HEAD~2 app.py   # restore a file to its state two commits ago
git clean -nd                   # show the untracked files that would be deleted (dry-run)
git clean -fd                   # delete them
```

### stash — put unfinished work aside
The situation: you're working on a feature and urgently need to switch to a hotfix, but you don't want to commit something half-done.
```bash
git stash push -m "wip: refactor config"     # hide the changes, the working copy is clean
git switch main                              # ... do the hotfix ...
git switch feature/x
git stash list                               # stash@{0}: On feature/x: wip: refactor config
git stash pop                                # bring back the latest one and remove it from the list
git stash apply stash@{1}                    # apply a specific one without removing it
git stash push -u                            # including new (untracked) files
```
An alternative for long switches is `git worktree` (see below).

### Fixing the last commit
```bash
git commit --amend -m "a better message"     # rewrite the last commit's message
git add forgotten.py && git commit --amend --no-edit   # add a forgotten file to the last commit
```
`--amend` creates a **new** commit in place of the last one. If the old one has already been pushed, you'll need `push --force-with-lease` — don't do this on shared branches.

### fetch and pull
- **`git fetch`** downloads new commits and updates `origin/*`. Your local branches and files **don't change**. A safe operation: you can look at what arrived before merging it in.
- **`git pull`** = `fetch` + `merge` (or `rebase` with `pull.rebase=true`) into the current branch.

```bash
git fetch --prune                  # --prune: remove origin/* for branches deleted on the server
git log --oneline HEAD..origin/main   # what arrived in origin/main that I don't have yet
```

### push --force and --force-with-lease
After a rebase or amend the branch history is rewritten, and an ordinary push is rejected (the remote branch is not an ancestor of the new one). A forced push is needed:
- `git push --force` overwrites the remote branch **no matter what**, even if a colleague managed to push something there. Their commits are lost.
- `git push --force-with-lease` overwrites it **only if** the remote branch is where you last saw it. If someone pushed something new, the push is refused. **Always use it instead of `--force`.**

### Tags
```bash
git tag -a v1.4.0 -m "Release 1.4.0"   # an annotated tag (with author and message) — for releases
git push origin v1.4.0                 # tags are not pushed automatically
git tag --sort=-v:refname | head       # the latest tags by version
git describe --tags                    # → v1.4.0-3-g9c4a1f2 — 3 commits after v1.4.0
```

### 🧪 Mini task

**Task.** Walk through Git's three areas: change a committed file and create a new one, look at `status --short` before and after `add`, unstage the file, discard the working copy changes. Then stash unfinished work together with the new file and bring it back.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo v1 > app.conf && git add app.conf && git commit -qm "add config"
echo v2 > app.conf; echo draft > notes.txt
git status --short
git add app.conf; git status --short
git restore --staged app.conf; git status --short
git restore app.conf; cat app.conf
echo v3 > app.conf
git stash push -q -u -m wip; git status --short; git stash list
git stash pop -q; git status --short
```

Expected output:

```text
 M app.conf
?? notes.txt
M  app.conf
?? notes.txt
 M app.conf
?? notes.txt
v1
stash@{0}: On main: wip
 M app.conf
?? notes.txt
```

Breakdown: `status --short` has two columns: the left is the index (staged), the right is the working copy. ` M` is modified but not added, `M ` is added to the index, `??` is an untracked file. `restore --staged` unstages without touching the file, while `restore` without a flag reverts the file itself to the committed version — `v2` is gone for good. After `stash -u` the status is empty (no line printed), because `-u` took the untracked `notes.txt` too. Without `-u` it would have stayed in the working copy.

</details>

## Viewing history
```bash
git log --oneline --graph --decorate --all   # graph of all branches
git log -p -- path/to/file                   # a file's history with changes
git log --follow -- file                     # history following renames
git log -S "connection_timeout"              # commits where a string appeared or disappeared (pickaxe)
git log -G "timeout\s*="                     # where lines matching a regular expression changed
git log --author="alex" --since="2 weeks ago" --until="yesterday"
git log --grep="JIRA-123"                    # by message text
git show <sha>                               # a commit with its changes
git show <sha>:path/file                     # a file as of that commit
git blame -L 40,60 app.py                    # who changed lines 40–60 and in which commit
git blame -w -C app.py                       # ignore whitespace, track code moved from other files
```
`log -S` is one of the most useful tricks: "when and by whom was this setting removed?" is found in seconds.

### 🧪 Mini task

**Task.** Make three commits touching two files. Find: the whole history as plain messages, the history of a single file, the commit that introduced the `timeout=30` line (search by content), and a summary of changes over the last two commits.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo "timeout=10" > app.conf && git add . && git commit -qm "add app config"
echo "replicas=2" > deploy.yaml && git add . && git commit -qm "add deploy"
echo "timeout=30" > app.conf && git commit -qam "raise timeout"
git log --format='%s'
echo ---; git log --format='%s' -- app.conf
echo ---; git log -S'timeout=30' --format='%s'
echo ---; git diff HEAD~2 --stat
```

Expected output:

```text
raise timeout
add deploy
add app config
---
raise timeout
add app config
---
raise timeout
---
 app.conf    | 2 +-
 deploy.yaml | 1 +
 2 files changed, 2 insertions(+), 1 deletion(-)
```

Breakdown: `--format='%s'` keeps only the subjects — handy in scripts and for a changelog. `-- app.conf` limits the history to that file. `-S` (pickaxe) finds commits where the *number of occurrences* of the string changed, i.e. where it was added or removed — that's how you find who changed a setting and when. `--stat` shows the size of the changes: one line replaced in `app.conf`, one added in `deploy.yaml`.

</details>

## Merge and rebase

Both commands solve the same task — **combining work from two branches** — but in different ways.

The starting point: `feature` branched off `main`, and both got new commits:
```
      C---D   feature
     /
A---B---E---F   main
```

### merge
```bash
git switch main
git merge feature
```
```
      C---D
     /     \
A---B---E---F---M   main   (M is a merge commit with two parents: F and D)
```
- History is preserved **as it was**: you can see that work went on in parallel.
- Commits C and D don't change (same hashes).
- A merge commit appears.
- Safe for shared branches.

**Fast-forward.** If `main` hasn't moved forward since the branch-off (there are no commits E and F), Git simply moves the `main` pointer to D, without a merge commit. `git merge --no-ff feature` always creates a merge commit: that way the feature's boundaries are visible in history. `--ff-only` refuses to merge if a fast-forward is impossible.

### rebase
```bash
git switch feature
git rebase main
```
```
A---B---E---F   main
             \
              C'---D'   feature
```
- Commits C and D are **replayed** on top of F. The result is **new** commits C' and D' (a different parent means a different hash).
- History becomes **linear**, as if the work had started from a fresh main.
- The old C and D remain in the repository without references (they're visible in the reflog).

After the rebase, `main` can be fast-forwarded, and history will be a straight line.

### The golden rule of rebase
**Don't rebase commits that are already published and that others base their work on.** Your colleagues will still have the old C and D, and you'll have the new C' and D' with the same changes. On the next pull everything gets mixed up: duplicate commits and conflicts appear.

You can and should rebase your own personal feature branch (to pull in a fresh main), followed by `push --force-with-lease`.

### What it looks like in practice
```bash
git switch feature/x
git fetch
git rebase origin/main          # replay my commits on top of a fresh main

# on a conflict rebase stops at the problematic commit:
git status                      # which files are in conflict
# fix the files, remove the markers <<<<<<< ======= >>>>>>>
git add app.py
git rebase --continue           # move on to the next commit
git rebase --abort              # changed your mind — put everything back as it was before the rebase
git rebase --skip               # drop the current commit (careful)

git push --force-with-lease
```

### Interactive rebase — tidying up history
Before a merge request you often need to turn "wip", "fix typo", "another fix" into neat commits:
```bash
git rebase -i main              # or HEAD~5 — the last 5 commits
```
An editor opens with a list of commits (the oldest at the top):
```
pick 1a2b3c4 feat: add healthcheck endpoint
pick 5d6e7f8 wip
pick 9a8b7c6 fix typo
pick 3c4d5e6 feat: add readiness probe
```
You change the commands and save:
- `pick` — keep as is;
- `reword` — keep, but change the message;
- `squash` — combine with the previous one, merging the messages;
- `fixup` — combine with the previous one, discarding this one's message;
- `drop` — delete the commit;
- `edit` — stop at the commit to change it (for example, to split it in two);
- you can reorder the lines — the order of commits changes.

Automation: you make a fix for a specific older commit with `git commit --fixup 1a2b3c4`, and then `git rebase -i --autosquash main` places the `fixup` lines in the right spots itself.

### Squash merge
The "Squash and merge" button in GitLab or GitHub: all the branch's commits become **one** commit in main. A clean main history (one commit per feature), but the branch's detailed history is lost in main.

### What to choose
| Situation | Approach |
|---|---|
| Pull a fresh main into your branch | `rebase` (linear, no extra merge commits) |
| Merge a feature into main | a merge request: `merge --no-ff` or squash — as agreed by the team |
| A shared long-lived branch (develop, release) | `merge` only |
| Tidy up before review | `rebase -i` |

### rebase --onto — moving part of a branch
The situation: the `feature-b` branch was created from `feature-a` instead of `main`. Now `feature-a` is not needed, and `feature-b` has to be moved onto `main` **without the commits of `feature-a`**:
```
A---B   main
     \
      C---D   feature-a
           \
            E---F   feature-b
```
```bash
git rebase --onto main feature-a feature-b
# "take the commits of feature-b that aren't in feature-a (that's E and F) and replay them on top of main"
```
```
A---B---E'---F'   feature-b
```

### 🧪 Mini task

**Task.** Create diverging branches: a `main fix` commit on `main` and a `feature work` commit on `feature`. Copy `feature` and rebase the copy onto `main`. Then merge the original `feature` into `main`. Compare the history of both branches and find the merge commit.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
git commit -q --allow-empty -m init
git switch -qc feature && git commit -q --allow-empty -m "feature work"
git switch -q main && git commit -q --allow-empty -m "main fix"
git switch -qc rebased feature && git rebase -q main
git log --format='%s' rebased
echo ---; git switch -q main && git merge -q --no-edit feature
git log --first-parent --format='%s' main
echo ---; git log --merges --format='%s' main
```

Expected output:

```text
feature work
main fix
init
---
Merge branch 'feature'
main fix
init
---
Merge branch 'feature'
```

Breakdown: rebase rewrote `feature work` on top of `main fix` — the history is linear, but it's a *new* commit with a different hash, so you must not rebase a branch that has been pushed and others work on. Merge kept both lines and added a merge commit with two parents. `--first-parent` shows history "through main's eyes" — only what was merged into it, handy for a changelog. `--merges` finds all merges.

</details>

## Conflicts

A conflict happens when both branches changed **the same lines** (or one changed a file and the other deleted it). Git can't decide for you and marks the spot:
```
<<<<<<< HEAD
timeout: 30
=======
timeout: 60
>>>>>>> feature/x
```
Between `<<<<<<<` and `=======` is the current branch's version, between `=======` and `>>>>>>>` is the incoming one. You keep the right variant (or combine both), remove the markers, then `git add` and continue (`git merge --continue` or `git rebase --continue`).

```bash
git diff --name-only --diff-filter=U    # list of files with conflicts
git checkout --ours config.yaml         # take "our" side's version entirely
git checkout --theirs config.yaml       # take "their" side's version
git mergetool                           # open a graphical merge tool
git merge --abort                       # cancel the whole merge
git log --merge -p -- config.yaml       # commits from both sides that touched the conflicting file
```

**The trap: during a rebase `ours` and `theirs` swap places.** During a rebase Git switches to the branch you're moving onto (main) and applies your commits one by one. So "ours" is main and "theirs" is your commits. With merge it's the other way round.

**rerere** (reuse recorded resolution) — Git remembers how you resolved a conflict and applies the resolution itself the next time the same conflict appears. Indispensable when you have to rebase a long-lived branch many times. Enabled with `git config --global rerere.enabled true`.

### 🧪 Mini task

**Task.** Create a conflict: change `timeout` to 30 on `main` and to 60 on `feature`, then merge. Look at the status and the conflict markers in the file, resolve it with the value 45 and finish the merge.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo "timeout=10" > app.conf && git add . && git commit -qm init
git switch -qc feature && echo "timeout=60" > app.conf && git commit -qam "timeout 60"
git switch -q main && echo "timeout=30" > app.conf && git commit -qam "timeout 30"
git merge feature
git status --short
cat app.conf
echo "timeout=45" > app.conf && git add app.conf && git commit -q --no-edit
git log -1 --format='%s'; cat app.conf
```

Expected output:

```text
Auto-merging app.conf
CONFLICT (content): Merge conflict in app.conf
Automatic merge failed; fix conflicts and then commit the result.
UU app.conf
<<<<<<< HEAD
timeout=30
=======
timeout=60
>>>>>>> feature
Merge branch 'feature'
timeout=45
```

Breakdown: Git can't choose between 30 and 60 itself, because both branches changed the same line relative to the common ancestor. `UU` means "modified in both branches, unmerged". Between `<<<<<<< HEAD` and `=======` is the current branch's version, below it is the incoming one. Resolving means leaving the right text without markers, `git add` and committing. If something goes wrong, `git merge --abort` restores everything as it was before the merge.

</details>

## Analysing heavily branched repositories

When a repository has dozens of branches, release branches and hotfixes, the main questions are: "what is in this branch that isn't in that one?", "did the fix make it into the release?", "which branches can be deleted?".

### Range syntax: two and three dots
```bash
git log main..feature       # commits that are in feature but NOT in main
                            # = "what merging feature into main will bring"
git log feature..main       # commits that are in main but not in feature
                            # = "how far feature has fallen behind main"
git log main...feature      # commits that are in only one of the branches (symmetric difference)
git log --left-right --oneline main...feature
# < a1b2c3 hotfix in main       ← '<' — only in the left one (main)
# > d4e5f6 new feature          ← '>' — only in the right one (feature)
```
With `git diff` the dots mean **something different** (a common source of confusion):
```bash
git diff main..feature      # the difference between the branch tips (the same as git diff main feature)
git diff main...feature     # what feature changed since branching off main
                            # = what a merge request shows. Changes in main are not counted
```

### Did a commit make it into a branch or release
```bash
git branch -a --contains a1b2c3d     # which branches (local and remote) contain this commit
git tag --contains a1b2c3d           # which releases (tags) contain it
git merge-base --is-ancestor a1b2c3d release/2.3 && echo "present in release/2.3"
```
The problem: after a cherry-pick the commit in the other branch has a **different hash**, and `--contains` won't see it. Then search by the content of the changes:
```bash
git cherry -v release/2.3 main
# + a1b2c3 fix: null pointer in auth     ← '+': this change is not in release/2.3
# - d4e5f6 fix: timeout                  ← '-': an equivalent patch is already there (moved via cherry-pick)
git log --cherry-pick --right-only --oneline release/2.3...main   # the same, only the missing ones
git log --all --grep="JIRA-123" --oneline                          # or by task number in the messages
```

### The common ancestor and a map of branches
```bash
git merge-base main feature                       # the commit where the branches diverged
git log --oneline --graph --all --simplify-by-decoration   # a "map": only commits with branches and tags
git show-branch main release/2.3 feature/x        # a table: which commits are in which branches
```

### Cleaning up branches
```bash
git branch --merged main                 # local branches already merged into main — can be deleted
git branch -r --merged origin/main       # the same for remote ones
git branch --no-merged main              # not merged yet — careful
git branch -d feature/old                # delete (refuses if not merged)
git branch -D feature/old                # force-delete
git push origin --delete feature/old     # delete on the server
# branches sorted by the date of the last commit: which ones have long been abandoned
git for-each-ref --sort=-committerdate refs/remotes/origin \
  --format='%(committerdate:short) %(authorname) %(refname:short)'
```
A limitation: `--merged` won't show branches merged via **squash merge**: main doesn't contain their commits, only one new one. Such branches are found via `git cherry` or by closed merge requests.

### Comparing two versions of a branch after a rebase
A reviewer wants to understand what changed between the previous and the new version of an MR after a rebase. A normal diff would also show all the changes in main. `range-diff` compares commits pairwise:
```bash
git range-diff main origin/feature@{1} feature
```

### 🧪 Mini task

**Task.** The `done` branch is already merged into `main`, the `wip` branch is not, and `main` got a hotfix after `wip` was created. Find the merged and unmerged branches, the commits missing from `main`, and count how far the branches have diverged in each direction.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
git commit -q --allow-empty -m init
git switch -qc done && git commit -q --allow-empty -m "done feature"
git switch -q main && git merge -q --no-ff --no-edit done
git switch -qc wip && git commit -q --allow-empty -m "wip 1" && git commit -q --allow-empty -m "wip 2"
git switch -q main && git commit -q --allow-empty -m hotfix
git branch --merged main
echo ---; git branch --no-merged main
echo ---; git log --format='%s' main..wip
echo ---; git rev-list --left-right --count main...wip
git merge-base --is-ancestor done main && echo "done is in main"
```

Expected output:

```text
  done
* main
---
  wip
---
wip 2
wip 1
---
1	2
done is in main
```

Breakdown: `--merged` lists branches whose commits are all in `main` — safe to delete; `--no-merged` lists branches with unmerged work. Two dots `main..wip` are the commits in `wip` but not in `main`. Three dots with `--left-right --count` give "1 on the left, 2 on the right": `main` has one commit missing from `wip` (the hotfix), and `wip` has two of its own. `merge-base --is-ancestor` answers "did this commit or branch make it into the release" with an exit code — handy in scripts.

</details>

## Moving changes between branches

### cherry-pick — copying a commit
A typical situation: a fix was made in `main`, and it has to be carried into the supported release branches.
```bash
git switch release/2.3
git cherry-pick -x a1b2c3d
# -x appends "(cherry picked from commit a1b2c3d)" to the message — later you can see where the fix came from
git cherry-pick A^..B            # a range of commits from A to B inclusive
git cherry-pick -m 1 <merge-sha> # move a merge commit (specify which parent to take the changes relative to)
git cherry-pick --continue / --abort   # on a conflict — as with rebase
```
cherry-pick creates a **new** commit with the same changes and a different hash.

### A hotfix into several releases: the options
1. **Into main first, then cherry-pick down** into the release branches. Simple and clear. The downside: one change becomes several different commits.
2. **Into the oldest supported branch first, then merge upwards** (`release/2.2` → `release/2.3` → `main`). There's one commit, and `--contains` shows it everywhere. Requires discipline.

### worktree — several branches at once
Instead of `stash` and switching back and forth, you can get a second working folder with another branch of the same repository:
```bash
git worktree add ../hotfix release/2.3   # ../hotfix holds a working copy of the release/2.3 branch
cd ../hotfix                             # ... do the hotfix while a build runs in the main folder ...
git worktree list
git worktree remove ../hotfix
```
Handy when a long build or tests are running in the main folder, or you need to compare the behaviour of two versions side by side.

### 🧪 Mini task

**Task.** After the `release/1.0` branch was cut, `main` got two commits: a new feature and a bug fix. Bring only the fix into the release branch with `cherry-pick -x`, and check that the feature didn't get there and that the message keeps a reference to the original commit.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo v1 > app.conf && git add . && git commit -qm "release 1.0"
git branch release/1.0
echo feature > feature.txt && git add . && git commit -qm "new feature"
echo fixed > app.conf && git commit -qam "fix crash on start"
git switch -q release/1.0
git cherry-pick -x main > /dev/null
git log --format='%s'
ls; cat app.conf
git log -1 --format='%b' | cut -c1-28
```

Expected output:

```text
fix crash on start
release 1.0
app.conf
fixed
(cherry picked from commit
```

Breakdown: `cherry-pick` copied into `release/1.0` only the changes of the `main` commit (the latest — the fix), and `feature.txt` didn't get into the release. It's a new commit with the same message but a different hash. The `-x` flag added the line "(cherry picked from commit <hash>)" to the body — later it shows where the hotfix came from. `cut` just trims the hash, which differs for everyone.

</details>

## Rescue and rollbacks

### reflog — the log of all movements
Git records every movement of HEAD and branches: commits, switches, reset, rebase, merge. The log is kept locally for about 90 days. **Almost anything that was committed can be brought back.**
```bash
git reflog
# 9c4a1f2 HEAD@{0}: reset: moving to HEAD~3
# 7f3e2d1 HEAD@{1}: commit: add retry logic        ← here is the lost commit
# 3b7e2d4 HEAD@{2}: commit: add healthcheck

git reset --hard 7f3e2d1            # put the branch back into that state
git branch rescue 7f3e2d1           # or safely: create a branch at the lost commit
git reflog show feature/x           # the log of a specific branch
```
What **cannot** be brought back: changes that were never committed or stashed (for example, after `git restore` or a `reset --hard` over uncommitted work).

### reset — moving a branch
`git reset` moves the pointer of the **current branch** to another commit. Three modes determine what happens to the index and the files:
```bash
git reset --soft HEAD~1    # the branch moves back one commit; the undone commit's changes STAY in the index
                           # → you can immediately commit differently (for example, combine several commits)
git reset --mixed HEAD~1   # (default) the changes stay in the working copy but not in the index
git reset --hard HEAD~1    # everything is discarded: both the commit and the changes in the files
git reset --hard origin/main   # make the local branch an exact copy of the remote one
```
reset **rewrites the branch's history**. It isn't suitable for shared branches that have already been pushed.

### revert — undoing a commit with a new commit
```bash
git revert a1b2c3d             # creates a new commit with the reverse changes
git revert -m 1 <merge-sha>    # undo a merge (-m 1: relative to the first parent — the main branch)
git revert --no-commit A..B    # undo a range in one commit
```
History isn't rewritten, so this is **the right way to roll back a change in a shared branch** (main, release).

A subtlety of reverting a merge: if you reverted a feature's merge and then fixed the feature and want to merge it again, Git decides that the feature's commits are already in main (they are there, their changes are just undone). You first need to **revert the revert commit itself**, and then merge the fixes.

### bisect — finding the commit that broke everything
A binary search through history. With 1000 commits between a "good" and a "bad" version, you'll find the culprit in ~10 steps.
```bash
git bisect start
git bisect bad                 # the current version is broken
git bisect good v2.3.0         # and everything worked at this tag
# Git switches to a commit in the middle. You check and answer:
git bisect good                # or git bisect bad
# ... repeat until Git says "a1b2c3d is the first bad commit"
git bisect reset               # go back to where you started

# automatically: the script must return 0 for "good", 1–127 (except 125) for "bad"
git bisect run ./scripts/test-health.sh
```

### 🧪 Mini task

**Task.** Make three commits and "lose" the last two with `reset --hard HEAD~2`. Find them in the `reflog` and bring the branch back. Then undo the last commit in a way that's safe for a shared branch — with `revert`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
for c in c1 c2 c3; do echo $c > $c.txt && git add . && git commit -qm $c; done
git reset -q --hard HEAD~2
git log --format='%s'
echo ---; git reflog --format='%gs' | head -3
git reset -q --hard 'HEAD@{1}'
echo ---; git log --format='%s'
git revert -q --no-edit HEAD
echo ---; git log --format='%s'; ls -1
```

Expected output:

```text
c1
---
reset: moving to HEAD~2
commit: c3
commit: c2
---
c3
c2
c1
---
Revert "c3"
c3
c2
c1
c1.txt
c2.txt
```

Breakdown: `reset --hard` moved the branch back; c2 and c3 vanished from `log` but not from the repository. The `reflog` is a local journal of every `HEAD` move: `HEAD@{1}` is the position before the last action, i.e. before the reset. Commits are kept at least 30 days until garbage collection removes them. `revert` doesn't rewrite history; it adds a new commit with the inverse changes: `c3.txt` is gone, while commit c3 remains. That's why shared branches are rolled back with `revert`, not with `reset` and `push --force`.

</details>

## Branching strategies

### GitFlow
Branches: `main` (releases only), `develop` (integration), `feature/*` (from develop), `release/*` (stabilisation before a release), `hotfix/*` (from main, merged into main and develop).
- Suitable for products with planned releases and support for several versions (boxed software, mobile apps).
- Downsides: a heavy process, long-lived branches, complicated merges, doesn't fit well with continuous delivery.

### GitHub Flow
`main` is always ready to deploy. For each task — a short branch, a merge request, review, CI, merge into main, deploy. Simple and suited to web services with continuous delivery.

### GitLab Flow
GitHub Flow plus environment branches (`main` → `staging` → `production`, changes are promoted by merging) or release branches for supported versions.

### Trunk-based development
Everyone integrates into `main` (the trunk) very often — at least once a day. Branches live for hours, not weeks. Unfinished features are hidden behind **feature flags**. Best suited to CI/CD: fewer conflicts, faster feedback. Requires good test coverage and a culture of small changes.

### Related practices
- **Protected branches** — you can't push to main directly, only via an MR with green CI and approval.
- **CODEOWNERS** — mandatory reviewers for paths (for example, `/terraform/prod/ @platform-team`).
- **Conventional Commits** (`feat:`, `fix:`, `chore:`, `feat!:` for breaking changes) — a convention for commit message format; it allows generating the CHANGELOG and version automatically (semantic-release — a tool that decides from the commit types itself: `fix` → patch version, `feat` → minor, `!` → major).
- **Signed commits** (GPG or SSH — the commit is signed with a personal key, and the platform shows a Verified mark) — proof of authorship: any name and email can be put into a commit, but a signature can't be forged.
- **pre-commit hooks** — linters, formatting, secret detection (gitleaks) before the commit.

### 🧪 Mini task

**Task.** Tag releases with annotated tags `v1.2.0`, `v1.9.0`, `v1.10.0` in release order, create a `release/1.10` branch from the latest tag, and find the latest tag reachable from the current commit. Compare plain and "version" sorting of the tags.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
for v in 1.2.0 1.9.0 1.10.0; do
  git commit -q --allow-empty -m "release $v" && git tag -a "v$v" -m "release $v"
done
git branch release/1.10 v1.10.0
git commit -q --allow-empty -m "next work"
git describe --tags --abbrev=0
echo ---; git tag --list
echo ---; git tag --sort=-v:refname
echo ---; git branch --list 'release/*'
```

Expected output:

```text
v1.10.0
---
v1.10.0
v1.2.0
v1.9.0
---
v1.10.0
v1.9.0
v1.2.0
---
  release/1.10
```

Breakdown: `describe --abbrev=0` found the nearest tag in the current commit's history — that's how CI derives the build version. Plain sorting is lexical, so `v1.10.0` came before `v1.2.0`. `--sort=-v:refname` compares as versions, descending — that's how you find "the latest release". A release branch from a tag is the basis of GitLab Flow and GitFlow: hotfixes are cherry-picked into it while development continues on `main`.

</details>

## Large repositories and monorepos
```bash
git clone --depth 1 <url>             # shallow: only the last commit (fast in CI)
git clone --filter=blob:none <url>    # partial: the history is there, file contents are fetched on demand
git sparse-checkout set services/api  # only the needed folders in the working copy
git submodule update --init --recursive   # pull in submodules
```
**Submodules** are a repository inside a repository, pinned to a specific commit. People don't like them: they're easy to forget to update, awkward in CI, and confusing for newcomers. Alternatives — packages and artifacts, versioned modules (Terraform modules by tag), a monorepo.

**Git LFS** — for large binary files: the repository holds a pointer, and the file itself lives in separate storage.

### 🧪 Mini task

**Task.** Make a monorepo with three commits and `services/api` and `services/web` folders. Clone it two ways: shallow (`--depth 1`) and partial (`--filter=blob:none --sparse`, only `services/api`). Compare how much history and how many files you get.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && git init -q -b main src && cd src
git config user.name dev && git config user.email dev@example.com
mkdir -p services/api services/web
echo api > services/api/main.go; echo web > services/web/index.js; echo docs > README.md
git add . && git commit -qm one
git commit -q --allow-empty -m two && git commit -q --allow-empty -m three
cd ..
git clone -q --depth 1 "file://$PWD/src" shallow
git -C src rev-list --count HEAD; git -C shallow rev-list --count HEAD
git clone -q --filter=blob:none --sparse "file://$PWD/src" sparse
git -C sparse sparse-checkout set services/api
ls -1 sparse; ls -1 sparse/services
```

Expected output:

```text
3
1
README.md
services
api
```

Breakdown: the shallow clone fetched only the latest commit — that's how CI jobs that don't need history are sped up (but `git describe` and `git log` are incomplete there). A partial clone with `--filter=blob:none` doesn't download file contents up front, and `sparse-checkout` keeps only the needed folders in the working copy: under `services` there is only `api`; root files (`README.md`) are kept by sparse mode by default. The `file://` scheme is needed for `--depth` and `--filter` to work with a local path too.

</details>

## Cases from interviews

**You committed to main instead of a feature branch and haven't pushed yet. What do you do?**
```bash
git branch feature/x            # the new branch points at the current commit — the work is saved
git reset --hard origin/main    # put main back where it is on the server
git switch feature/x            # continue working in the right branch
```

**You did a `reset --hard` and lost commits.**
`git reflog`, find the hash of the needed state, `git reset --hard <sha>` or `git branch rescue <sha>`. Works for everything that was committed.

**A password was accidentally committed and pushed. What do you do?**
1. **Rotate the secret first.** It's already compromised: the repository may have been cloned, CI may have logged it, and on GitHub bots find it within minutes. Removing it from history doesn't undo that.
2. Check the access logs: has the secret been used.
3. Remove it from history: `git filter-repo --path config/secrets.env --invert-paths` (filter-repo and BFG Repo-Cleaner are tools that rewrite the whole repository history, cutting a file or string out of every commit), then `push --force` to all branches and tags; ask everyone to re-clone; on GitHub/GitLab clean up caches and references from MRs.
4. Prevention: pre-commit with gitleaks, secret scanning on the platform (GitHub and GitLab check every push for known key formats themselves and can block it), secrets in Vault, Secrets Manager or CI variables.

**How do you undo an already pushed commit in a shared branch?**
`git revert <sha>` and an ordinary push. Not `reset` + `push --force`: that rewrites history for everyone.

**How does `git fetch` differ from `git pull`?**
`fetch` only downloads new commits and updates `origin/*` without changing anything locally. `pull` = `fetch` + `merge` (or `rebase`) into the current branch.

**How do `reset`, `revert` and `restore` differ?**
`reset` moves the branch pointer (rewrites history; the soft, mixed and hard modes determine what happens to the index and files). `revert` creates a new commit that undoes the changes (history isn't rewritten). `restore` reverts files in the working copy or index without touching commits.

**Merge or rebase — which and when?**
Rebase — to pull a fresh main into your personal branch, get a linear history, tidy up commits before review. Merge — to combine shared branches and merge a feature into main. Rebase rewrites hashes, so it isn't suitable for published shared branches.

**How does `--force` differ from `--force-with-lease`?**
`--force-with-lease` refuses to overwrite the remote branch if commits appeared there that you didn't see at the last fetch. It won't destroy someone else's work.

**A hotfix has to be carried into three release branches. How?**
Fix it in main (or in the oldest supported branch) and `git cherry-pick -x` into the rest. To verify: `git branch --contains` or, for cherry-picked ones, `git cherry -v release/2.3 main`. The alternative is fixing the oldest branch and merging up the chain.

**How do you find the commit after which the build broke?**
`git bisect`: mark a good and a bad version, and Git switches commits using binary search. Automate with `git bisect run ./test.sh`.

**How do you tell whether a fix made it into a release?**
`git tag --contains <sha>` or `git branch -a --contains <sha>`. If the fix was moved by cherry-pick (a different hash) — `git cherry` or a search by task number `git log --all --grep=JIRA-123`.

**How do you see what will change when merging feature into main?**
`git log main..feature` — which commits will arrive. `git diff main...feature` (three dots) — the changes in feature relative to the branch-off point, i.e. exactly what a merge request shows.

**What is a detached HEAD and why is it dangerous?**
HEAD points directly at a commit instead of a branch (after checking out a hash or a tag). New commits don't belong to any branch and are easy to lose after switching. To keep them — `git switch -c new-branch`.

**What is a fast-forward merge?**
A merge where the target branch hasn't moved ahead: the pointer simply moves to the last commit of the incoming branch, and no merge commit is created. `--no-ff` always creates a merge commit.

**How do you tidy up history before a merge request?**
`git rebase -i main`: combine small commits (`squash`, `fixup`), rewrite messages (`reword`), remove the unnecessary ones (`drop`). Then `git push --force-with-lease`.

**You added a file to `.gitignore`, but Git keeps tracking it.**
`.gitignore` only applies to untracked files. You need to remove the file from the index while keeping it on disk: `git rm --cached file`, then commit.

**A conflict appeared during a rebase, and `--ours` took the wrong thing. Why?**
During a rebase the sides swap: "ours" is the branch you're moving onto (main), "theirs" is your commits being moved.

**A feature's merge was reverted, the feature was fixed, and merging it again brings nothing. Why?**
The feature's commits are already in main's history; their changes are just undone by the revert commit. You first need to revert that revert commit, and then merge the fixes.

**Why aren't remote branches shown by `git branch --merged` even though they were merged?**
They were merged via squash merge: one new commit appeared in main, but the branch's commits aren't there. Such branches are found via `git cherry` or by closed merge requests.

**A branch was deleted. How do you restore it?**
It depends on where it was deleted:
- **Deleted locally** (`git branch -D feature/x`), and the commits weren't pushed. A branch is just a pointer; the commits themselves are still in the repository. Find the branch's last commit in the reflog and create the branch again:
  ```bash
  git reflog | grep feature/x        # a line like "checkout: moving from feature/x to main" — the hash before it
  git branch feature/x a1b2c3d       # or: git switch -c feature/x a1b2c3d
  ```
  On deletion Git itself prints `Deleted branch feature/x (was a1b2c3d)` — that hash works too.
- **Deleted locally, but the branch exists on the server** — just create a local copy:
  ```bash
  git fetch --all --prune
  git ls-remote --heads origin feature/x       # make sure the branch exists on the server
  git switch -c feature/x origin/feature/x     # or simply: git switch feature/x
  ```
- **Deleted on the server** (`git push origin --delete` or after a merge), and you or a colleague have a local copy — push it back: `git push -u origin feature/x`. If nobody has a copy — in GitLab and GitHub the commits of a merged branch remain in the merge request history and in the target branch; you can find the hash of the last commit in the MR and create a branch from it.
