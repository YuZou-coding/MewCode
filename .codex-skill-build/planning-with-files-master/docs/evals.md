# Benchmark Results: planning-with-files

Formal evaluation of `planning-with-files` using Anthropic's [skill-creator](https://github.com/anthropics/skills/tree/main/skills/skill-creator) framework, plus later functional verification of v3-specific mechanisms. This document records the full methodology, test cases, grading criteria, and results. Tests 1 through 3 were run against v2.22.0 (2026-03-06); Test 4 was run against v3.2.0 (2026-07-03).

---

## Why We Did This

A proactive security audit in March 2026 identified a prompt injection amplification vector in the hook system. The PreToolUse hook re-reads `task_plan.md` before every tool call — the mechanism that makes the skill effective — but declaring `WebFetch` and `WebSearch` in `allowed-tools` created a path for untrusted web content to reach that file and be re-injected into context on every subsequent tool use.

Hardened in v2.21.0: removed `WebFetch`/`WebSearch` from `allowed-tools`, added explicit Security Boundary guidance to SKILL.md. These evals document the performance baseline and verify zero regression in workflow fidelity.

---

## Test Environment

| Item | Value |
|------|-------|
| Skill version tested | 2.21.0 |
| Eval framework | Anthropic skill-creator (github.com/anthropics/skills) |
| Executor model | claude-sonnet-4-6 |
| Eval date | 2026-03-06 |
| Eval repo | Local copy (planning-with-files-eval-test/) |
| Subagents | 10 parallel (5 with_skill + 5 without_skill) |
| Comparator agents | 3 blind A/B comparisons |

---

## Test 1: Evals + Benchmark

### Skill Category

`planning-with-files` is an **encoded preference skill** (not capability uplift). Claude can plan without the skill — the skill encodes a specific 3-file workflow pattern. Assertions test workflow fidelity, not general planning ability.

### Test Cases (5 Evals)

| ID | Name | Task |
|----|------|------|
| 1 | todo-cli | Build a Python CLI todo tool with persistence |
| 2 | research-frameworks | Research Python testing frameworks, compare 3, recommend one |
| 3 | debug-fastapi | Systematically debug a TypeError in FastAPI |
| 4 | django-migration | Plan a 50k LOC Django 3.2 → 4.2 migration |
| 5 | cicd-pipeline | Create a CI/CD plan for a TypeScript monorepo |

Each eval ran two subagents simultaneously:
- **with_skill**: Read `SKILL.md`, follow it, create planning files in output dir
- **without_skill**: Execute same task naturally, no skill or template

### Assertions per Eval

All assertions are **objectively verifiable** (file existence, section headers, field counts):

| Assertion | Evals |
|-----------|-------|
| `task_plan.md` created in project directory | All 5 |
| `findings.md` created in project directory | Evals 1,2,4,5 |
| `progress.md` created in project directory | All 5 |
| `## Goal` section in task_plan.md | Evals 1,5 |
| `### Phase` sections (1+) in task_plan.md | All 5 |
| `**Status:**` fields on phases | All 5 |
| `## Errors Encountered` section | Evals 1,3 |
| `## Current Phase` section | Eval 2 |
| Research content in `findings.md` (not task_plan.md) | Eval 2 |
| 4+ phases | Eval 4 |
| `## Decisions Made` section | Eval 4 |

**Total assertions: 30**

### Results

| Eval | with_skill | without_skill | with_skill files | without_skill files |
|------|-----------|---------------|-----------------|---------------------|
| 1 todo-cli | 7/7 (100%) | 0/7 (0%) | task_plan.md, findings.md, progress.md | plan.md, todo.py, test_todo.py |
| 2 research | 6/6 (100%) | 0/6 (0%) | task_plan.md, findings.md, progress.md | framework_comparison.md, recommendation.md, research_plan.md |
| 3 debug | 5/5 (100%) | 0/5 (0%) | task_plan.md, findings.md, progress.md | debug_analysis.txt, routes_users_fixed.py |
| 4 django | 5/6 (83.3%) | 0/6 (0%) | task_plan.md, findings.md, progress.md | django_migration_plan.md |
| 5 cicd | 6/6 (100%) | 2/6 (33.3%) | task_plan.md, findings.md, progress.md | task_plan.md (wrong structure) |

**Aggregate:**

| Configuration | Pass Rate | Total Passed |
|---------------|-----------|-------------|
| with_skill | **96.7%** | 29/30 |
| without_skill | 6.7% | 2/30 |
| **Delta** | **+90.0 pp** | +27 assertions |

**Timing and token usage** (from task completion notifications — captured at runtime):

| Eval | with_skill tokens | with_skill time | without_skill tokens | without_skill time |
|------|------------------|-----------------|---------------------|-------------------|
| 1 todo-cli | 17,802 | 99.7s | 13,587 | 76.2s |
| 2 research | 22,150 | 128.7s | 13,610 | 127.3s |
| 3 debug | 17,506 | 93.4s | 11,525 | 66.5s |
| 4 django | 24,049 | 147.9s | 12,351 | 141.4s |
| 5 cicd | 18,122 | 105.0s | 8,424 | 76.7s |
| **Average** | **19,926** | **115s** | **11,899** | **98s** |

The skill uses ~68% more tokens and ~17% more time on average. The extra cost is the structured output: creating 3 files instead of 1-2, following phase/status discipline, populating decisions and error tables. This is the intended tradeoff — the skill trades speed for structure.

#### One Assertion Refined (Eval 4)

Assertion: `**Status:** pending on at least one future phase`
Result: not met

The agent completed all 6 migration phases in a single comprehensive planning session, leaving none pending. The skill was followed correctly — the assertion was overly prescriptive. The skill does not require phases to remain pending; it requires phases to have status fields. Revised for future evals: `task_plan.md contains **Status:** fields` (without specifying value).

---

## Test 2: A/B Blind Comparison

Three independent comparator agents evaluated pairs of outputs **without knowing which was with_skill vs without_skill**. Assignment was randomized:

| Eval | A | B | Winner | A score | B score |
|------|---|---|--------|---------|---------|
| 1 todo-cli | without_skill | with_skill | **B (with_skill)** | 6.0/10 | 10.0/10 |
| 3 debug-fastapi | with_skill | without_skill | **A (with_skill)** | 10.0/10 | 6.3/10 |
| 4 django-migration | without_skill | with_skill | **B (with_skill)** | 8.0/10 | 10.0/10 |

**with_skill wins: 3/3 = 100%**

### Comparator Quotes

**Eval 1 (todo-cli):** *"Output B satisfies all four structured-workflow expectations precisely... Output A delivered real, runnable code (todo.py + a complete test suite), which is impressive, but it did not fulfill the structural expectations... Output A's strength is real but out of scope for what was being evaluated."*

**Eval 3 (debug-fastapi):** *"Output A substantially outperforms Output B on every evaluated expectation. Output B is a competent ad-hoc debug response, but it does not satisfy the structured, multi-phase planning format the eval specifies. Output A passes all five expectations; Output B passes one and fails four."*

**Eval 4 (django-migration):** *"Output B is also substantively strong: it covers pytz/zoneinfo migration (a 4.2-specific item Output A omits entirely), includes 'django-upgrade' as an automated tooling recommendation... The 18,727 output characters vs 12,847 for Output A also reflects greater informational density in B."*

---

## Test 3: Description Optimizer

**Status: Not run in this cycle**

Requires `ANTHROPIC_API_KEY` in the eval environment. Per the project's eval standards, a test is only included in results if it can be run end-to-end with verified metrics. Scheduled for the next eval cycle.

---

## Test 4: v3 Long-Running Session Functional Verification (2026-07-03)

Tests 1 through 3 measure whether the skill enforces the 3-file planning pattern. They predate v3.0.0 (2026-06-09) and say nothing about v3's own headline feature: surviving `/clear` and context compaction across a long-running session. This section covers that mechanism specifically, added after a repository health audit found it was silently broken on Windows.

### Method

Rather than reading the code and assuming it worked, each mechanism was run directly in a scratch directory on a real Windows machine (Git Bash and PowerShell, both), with real files and real tampering, checking actual command output against expectations.

### Scope

| Mechanism | Script | What it does |
|-----------|--------|---------------|
| Session init | `init-session.sh` / `.ps1` | Creates `task_plan.md`, `findings.md`, `progress.md` from templates |
| Phase status | `check-complete.sh` / `.ps1` | Counts phases, reports completion |
| Attestation | `attest-plan.sh` / `.ps1` | SHA-256 locks plan content, detects tampering |
| Plan injection | `inject-plan.sh` | Re-injects plan context into the model turn, enforces the attestation |
| Parallel plans | `resolve-plan-dir.sh` / `.ps1` | Resolves the active plan directory across concurrent sessions |
| Session recovery | `session-catchup.py` | Reconstructs unsynced context after `/clear`, the actual "long-running session" mechanism |

### Result: 2 of 6 mechanisms were broken on Windows, silently

**`session-catchup.py` did nothing on Windows.** Two independent bugs, either one alone sufficient to break it:

1. The path sanitizer only replaced forward slashes. A Windows path (`C:\Users\...` or Git Bash's `/c/Users/...`) never matched Claude's actual project-directory naming, so the function always returned before finding any sessions to scan.
2. Three file reads had no explicit encoding, so any session log containing non-ASCII text raised `UnicodeDecodeError` against Windows' default `cp1252` codec, an error the surrounding `except` clauses swallowed without a trace.

Neither failure printed anything. A user running this on Windows would see no catchup report and have no reason to suspect the mechanism was even running, let alone why it produced nothing. `tests/test_path_fix.py` contained a working reimplementation of the correct fix but never imported or exercised the actual shipped script, so the test suite reported green the entire time.

**`inject-plan.sh`'s containment guard silently dropped plan injection under aliased paths.** The guard canonicalizes the project root and the candidate plan directory separately, then checks that one is a prefix of the other. On a Windows account with an 8.3 short-name `TEMP` (this test machine's own account is one such case) or a path reached through the MSYS `/tmp` mount, the two canonicalization routes land on differently-spelled versions of the same directory, the prefix check fails, and the hook exits with zero output: no plan re-injection, no tamper warning, nothing. `resolve-plan-dir.sh` does not have this bug because it builds its candidates as absolute paths from the start; that asymmetry is what pointed at the fix.

### What worked correctly, unmodified

Session init, phase-status counting, attestation lock/show/clear, tamper detection logic itself (once the injection guard reaches it), and parallel-plan directory resolution (`$PLAN_ID` env var, `.active_plan` file, newest-mtime fallback) all produced correct output in both Git Bash and PowerShell, including the v3 autonomous-mode chain (nonce-framed delimiters, unattested-plan refusal, per-tool-call injection suppression).

One asymmetry noted but not a bug: `init-session.ps1` has no slug mode (it always writes to the project root), only `init-session.sh` creates `.planning/<slug>/` directories. And there is no `inject-plan.ps1`; the injection and tamper-enforcement hook body is sh-only, so a pure-PowerShell host without Git Bash gets no plan injection or tamper enforcement at all.

### Fixed in v3.2.0

Both `session-catchup.py` and `inject-plan.sh` were fixed (see CHANGELOG). Re-running the same sequence after the fix confirmed session-catchup now produces a correct catchup report from real session logs, and plan injection now reaches the tamper-check branch under the same aliased-path conditions that previously went silent. The PowerShell-only injection gap and the `init-session.ps1` slug-mode asymmetry are open follow-ups, not addressed in this cycle.

### A caveat on what session-catchup actually does, working or not

Even fixed, `session-catchup.py` replays conversation transcript from the previous session; it does not parse or reconstruct the on-disk phase state directly. Its own "RECOMMENDED" output tells the agent to go read `task_plan.md`, `progress.md`, and `findings.md` itself. It is a conversation catchup that points at the files, not a plan-state reconstruction, and its usefulness is bounded by how much relevant discussion happened after the last planning-file edit.

---

## Summary

| Test | Status | Result |
|------|--------|--------|
| Evals + Benchmark | ✅ Complete | 96.7% (with_skill) vs 6.7% (without_skill) |
| A/B Blind Comparison | ✅ Complete | 3/3 wins (100%) for with_skill |
| Description Optimizer | Pending | Scheduled for next eval cycle |
| v3 Long-Running Session Functional Verification | ✅ Complete (2026-07-03) | 2 of 6 mechanisms found broken on Windows and fixed in v3.2.0; see Test 4 |

The skill demonstrably enforces the 3-file planning pattern across diverse task types. Without the skill, agents default to ad-hoc file naming and skip the structured planning workflow entirely. Separately, v3's session-recovery mechanism is now verified functional on Windows as of v3.2.0; it was not before, and nothing in the test suite would have caught that on its own.

---

## Reproducing These Results

```bash
# Clone the eval framework
gh api repos/anthropics/skills/contents/skills/skill-creator ...

# Set up workspace
mkdir -p eval-workspace/iteration-1/{eval-1,eval-2,...}/{with_skill,without_skill}/outputs

# Run with_skill subagent
# Prompt: "Read SKILL.md at path X. Follow it. Execute: <task>. Save to: <output_dir>"

# Run without_skill subagent
# Prompt: "Execute: <task>. Save to: <output_dir>. No skill or template."

# Grade assertions, produce benchmark.json
# See eval-workspace/iteration-1/benchmark.json for full data
```

Raw benchmark data: [`eval-workspace/iteration-1/benchmark.json`](../planning-with-files-eval-test/eval-workspace/iteration-1/benchmark.json) (in eval-test copy, not tracked in main repo)
