# SP6: Minor Packages + Enforcement — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Achieve ≥60% coverage for `jira` and `admin` packages, ≥80% for `middleware`. Verify enforcement infrastructure (Makefile, check-coverage.sh) already in place from SP1.

**Architecture:** jira — test pure parsing functions (tryParseSprint, tryParseSprintValue, extractSprintField, extractCustomFields, IsNotFound). admin — test RotateNow, generatePassword, RepoAdapter with function-field mocks. middleware — add trivial tests for UserCargoFromContext, ContextWithEquipeIDs, ValidateEquipeAccess.

**Tech Stack:** Go stdlib `testing`, `net/http/httptest` (for jira HTTP tests), `go.uber.org/zap`

## Global Constraints

- Framework: stdlib `testing` only
- No commits until everything passes 100%
- Enforcement already done in SP1 (Makefile targets, check-coverage.sh, .gitignore)

---

### Task 1: jira package tests + admin package tests + middleware boosters

**Files:**
- Modify: `internal/jira/client_test.go` (add tests for uncovered functions)
- Create: `internal/admin/rotation_test.go`
- Modify: `internal/middleware/auth_test.go` or `equipe_filter_test.go` (add trivial tests)

### Task 2: Final coverage verification + enforcement check
