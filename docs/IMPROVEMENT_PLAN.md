# IMPROVEMENT PLAN & ROADMAP — Django Recon Application

## Phase 1 — Stability, Safety & Correctness (COMPLETED)

All Phase 1 improvements have been implemented and verified:
- [x] **HTTP Server Timeouts & Graceful Shutdown**: `main.go` signal handling and timeout configuration.
- [x] **Input Validation**: `isValidTargetDomain()` preventing CLI flag injection and SSRF/internal network targeting.
- [x] **Dead Code Cleanup**: Removed `infoScore()`.
- [x] **State Machine Hardening**: `ScanJob` status constants defined in `models/scanjob.go`.
- [x] **Parser Side-Effect Removal**: Removed `os.Remove()` from `ParseHTTPXLine`.
- [x] **Worker Context Cancellation**: Fixed context inheritance in worker job execution.
- [x] **Notifier Queue Concurrency**: Fixed race condition in queue `Stop()`.
- [x] **Memory Cap**: Pipeline step output buffer capped at 50,000 items.
- [x] **StringArray Driver Consistency**: Fixed `StringArray.Value()` return type consistency.
- [x] **GORM Log Level**: Environment-based log level configuration.
- [x] **DevOps Basics**: Created `.gitignore`.

---

## Phase 2 — Scalability & Maintainability (RECOMMENDED NEXT STEPS)

### 1. Dashboard Authentication Middleware
- **Priority**: High
- **Impact**: High (prevents unauthorized access)
- **Effort**: Low (1-2 hours)
- **Files**: `internal/api/middleware.go`, `internal/config/config.go`
- **Description**: Add API token check (`X-API-Token` or query param) or HTTP Basic Auth when deployed on non-localhost interfaces.

### 2. Finding Fingerprinting & Deduplication
- **Priority**: Medium
- **Impact**: High (prevents duplicate Nuclei findings on re-scans)
- **Effort**: Medium (2-3 hours)
- **Files**: `internal/models/finding.go`, `internal/pipeline/pipeline.go`
- **Description**: Generate a SHA-256 fingerprint from `(target_id, tool_name, title, raw_output)` and use GORM `ON CONFLICT DO NOTHING`.

### 3. Shared Filter Builder in API Handlers
- **Priority**: Medium
- **Impact**: Medium (reduces code duplication in `handlers.go`)
- **Effort**: Low (1-2 hours)
- **Files**: `internal/api/handlers.go`
- **Description**: Extract shared GORM query filter logic used in `buildSubdomainPageData` and `buildGalleryPageData`.

---

## Phase 3 — Feature Enhancements (PRODUCT ROADMAP)

### 1. Asset Change Detection (Delta View)
- **VALUE**: High | **EFFORT**: Medium
- Track newly discovered subdomains and ports between consecutive scans for the same target.

### 2. Result Export (JSON / CSV / SARIF)
- **VALUE**: High | **EFFORT**: Low
- Add export endpoints `/api/export/subdomains` and `/api/export/findings` for reporting.

### 3. Tool Health Check Startup Routine
- **VALUE**: Medium | **EFFORT**: Low
- Verify presence and versions of `subfinder`, `dnsx`, `httpx`, `naabu`, `nuclei`, `katana` at application startup and display warnings in dashboard if missing.
