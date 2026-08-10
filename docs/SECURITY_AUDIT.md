# SECURITY AUDIT REPORT — Django Recon Application

## 1. Attack Surface Analysis

The application exposes:
1. **Web Dashboard & REST API**: HTTP endpoints running on port `8080` (or configured `PORT`).
2. **Subprocess Execution Engine**: Directly invokes binary tools (`subfinder`, `dnsx`, `httpx`, `naabu`, `nuclei`, `katana`).
3. **Telegram Bot Integration**: Outbound HTTPS requests to `https://api.telegram.org`.
4. **SQLite Database**: Local file storage (`recon.db`).

---

## 2. OWASP & Application Security Assessment

### A. Command Injection
- **Risk**: User submits target domain string -> passed as CLI argument to subfinder/httpx/naabu.
- **Analysis**: Subprocess execution uses `exec.CommandContext(ctx, name, args...)` without invoking a shell (`sh -c` / `bash -c`). This prevents shell metacharacter injection (`|`, `;`, `&`, `$()`).
- **Remediation Applied**: Added `isValidTargetDomain()` in `handlers.go` to explicitly reject domain strings starting with `-` (preventing CLI flag injection attacks such as `--config`, `-o`, etc.).

### B. Target Scope & SSRF Controls
- **Risk**: User inputs internal network IPs or cloud metadata endpoints (`169.254.169.254`, `127.0.0.1`, `10.0.0.1`) causing active scanning of internal infrastructure.
- **Remediation Applied**: `isValidTargetDomain()` rejects:
  - Loopbacks (`localhost`, `127.0.0.1`, `::1`)
  - Private IP spaces (RFC1918: `10.x.x.x`, `192.168.x.x`)
  - Link-local addresses (`169.254.x.x`)

### C. Authentication & Authorization
- **Current State**: Unauthenticated by default.
- **Recommendation**: For production deployment outside `localhost`, implement API Token authentication (`X-API-Token`) or HTTP Basic Auth via middleware.

### D. File System & Path Traversal
- **Remediation Applied**:
  - Removed `os.Remove()` from `ParseHTTPXLine` parser.
  - Sanitized screenshot paths in `handleScreenshotModal`.

### E. HTTP Server Protection (DoS)
- **Remediation Applied**: Added strict server-level timeouts:
  - `ReadHeaderTimeout`: 5s
  - `ReadTimeout`: 15s
  - `WriteTimeout`: 30s
  - `IdleTimeout`: 60s
  - `MaxHeaderBytes`: 1MB
