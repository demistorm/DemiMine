# Fix WebSocket Log Stream Corruption

## Problem

The log streaming system is corrupting log messages, showing:
- Extra characters like `K` at the beginning of lines
- Truncated log lines (only showing the last few characters)
- Mixed/incomplete log content

## Root Cause

The `StreamLogs()` function in `manager/internal/docker/client.go:82-113` is reading Docker's multiplexed log stream directly without proper decoding. Docker's `ContainerLogs` API returns a multiplexed stream where each log frame has:
- 8 bytes header (1 byte stream type + 3 reserved bytes + 4 bytes length)
- Then the actual log content

This 8-byte header is being treated as part of the log content, causing corruption.

## Solution

Use Docker's `stdcopy` package to properly decode the multiplexed stream.

### Implementation

1. **Add import** for `stdcopy` package:
   ```go
   "github.com/docker/docker/pkg/stdcopy"
   ```

2. **Replace `StreamLogs()` implementation**:
   - Create a custom `io.Writer` that sends decoded log lines to the channel
   - Use `stdcopy.StdCopy()` to decode the multiplexed stream
   - Handle both stdout and stderr streams

### Code Changes

**File:** `manager/internal/docker/client.go`

1. Add `stdcopy` import
2. Replace `StreamLogs()` function with proper decoded implementation

## Testing

After applying changes, rebuild and restart:
```bash
docker compose build --no-cache && docker compose up -d manager
```

Verify that:
- Log lines appear complete without extra characters
- No truncation at beginning or end
- All log content displays correctly in real-time
