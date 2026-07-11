---
name: pr-review-toolkit
description: Manual PR and code review workflow adapted from Anthropic Claude Code pr-review-toolkit for reviewing diffs, pull requests, branches, commits, changed files, tests, silent failures, comments, type design, and simplification opportunities. Use only when the user explicitly asks to use pr-review-toolkit, code review skill, PR review toolkit, or this specific installed skill by name.
---

# PR Review Toolkit

Use this skill to perform a focused pull request or code change review. Default to a code-review stance: findings first, ordered by severity, grounded in file and line references, with concise remediation guidance.

This skill is adapted for Codex from Anthropic's Claude Code `pr-review-toolkit` plugin:
https://github.com/anthropics/claude-code/tree/main/plugins/pr-review-toolkit

## Review Workflow

1. Establish the review target.
   - Prefer the current diff if the user does not specify a PR, branch, commit range, or files.
   - Inspect repository status before reading code.
   - Identify changed files and relevant surrounding code before judging behavior.

2. Review for correctness before style.
   - Look for bugs, regressions, unsafe assumptions, edge cases, data loss, race conditions, security issues, and missing validation.
   - Treat tests as evidence, not decoration. Check whether changed behavior has meaningful tests and whether existing tests could pass while the bug remains.

3. Use the specialist passes when relevant.
   - Code reviewer: behavioral defects, risky architecture changes, missing tests, API contract breaks.
   - Comment analyzer: stale, misleading, redundant, or absent comments where intent is non-obvious.
   - PR test analyzer: weak assertions, untested branches, brittle fixtures, skipped tests, false confidence.
   - Silent failure hunter: swallowed errors, ignored return values, fallback paths that hide broken behavior, logs without action.
   - Type design analyzer: overly broad types, invalid states expressible in types, unclear ownership, weak domain modeling.
   - Code simplifier: needless abstraction, duplicate logic, confusing control flow, code that can be removed without behavior loss.

4. Verify claims.
   - Run targeted tests or static checks when practical.
   - If commands are unavailable or unsafe, state exactly what was not run and why.
   - Do not report speculative issues as facts; label uncertainty clearly.

## Output Format

Return findings first. If there are no actionable findings, say so clearly and mention residual risk or missing verification.

For each finding include:

- Severity: `P0`, `P1`, `P2`, or `P3`.
- Location: exact file and line when available.
- Problem: the concrete failure mode.
- Impact: what can go wrong for users, data, operations, or maintainers.
- Fix direction: short, practical guidance.

After findings, include open questions or assumptions only if they affect review confidence. Keep summaries secondary and brief.

## Severity Guide

- `P0`: breaks builds, causes data loss, introduces exploitable security flaws, or blocks core workflows.
- `P1`: likely user-visible regression, production failure, authorization flaw, or serious correctness issue.
- `P2`: real defect or meaningful test gap with bounded impact.
- `P3`: maintainability, clarity, or low-risk robustness issue.

## Review Discipline

- Do not rewrite the whole change unless the user asks for implementation.
- Do not bury findings under compliments.
- Do not invent line numbers. If exact lines are unavailable, cite the file and nearest symbol or changed hunk.
- Prefer one strong finding over many weak style notes.
- Mention source compatibility issues when adapting advice from Claude plugin behavior to Codex.
