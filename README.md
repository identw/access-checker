# Access Checker

A TCP server/client application for testing network throughput with hash validation.

## Features

- **Multi-port support**: Server listens on multiple TCP ports simultaneously
- **Two operation modes**:
  - **Download**: Client requests random data from server, server sends data + SHA256 hash
  - **Upload**: Client sends data + SHA256 hash to server, server validates
- **Configurable**: Ports can be set via command-line flag or environment variable
- **Data validation**: All operations include SHA256 hash verification
- **Size limit**: Maximum 16MB per operation
- **Test automation**: Run multiple tests from YAML configuration

## Project Structure

```
access-checker/
├── cmd/
│   ├── server/          # Server application
│   │   └── main.go
│   └── client/          # Client application
│       └── main.go
├── pkg/
│   └── protocol/        # Shared protocol constants
│       └── protocol.go
├── config.yaml.example  # Example configuration file
├── go.mod              # Go module
└── README.md
```

## Protocol

### Message Format

**Download Request (Client → Server):**
```
1 byte  - message type (1 = download)
4 bytes - data size (uint32, big endian)
```

**Download Response (Server → Client):**
```
4 bytes  - data size (uint32, big endian)
N bytes  - random data
32 bytes - SHA256 hash of data
```

**Upload Request (Client → Server):**
```
1 byte   - message type (2 = upload)
4 bytes  - data size (uint32, big endian)
N bytes  - data
32 bytes - SHA256 hash of data
```

**Upload Response (Server → Client):**
```
1 byte - result (0 = fail, 1 = success)
```

## Building

```bash
# Build server
go build -o server ./cmd/server

# Build client
go build -o client ./cmd/client

# Or build both
go build -o server ./cmd/server && go build -o client ./cmd/client
```

## Running the Server

### Default ports (8080, 8081, 8082):
```bash
./server
# or
go run ./cmd/server
```

### Custom ports via flag:
```bash
./server -ports 3000,3001,3002
# or
go run ./cmd/server -ports 3000,3001,3002
```

### Custom ports via environment variable:
```bash
PORTS=9000,9001,9002 ./server
# or
PORTS=9000,9001,9002 go run ./cmd/server
```

**Priority**: command-line flag > environment variable > default values

## Running the Client

The client supports two modes of operation:
1. **Command-line flags** - for single ad-hoc tests
2. **Configuration file** - for running multiple tests with different parameters

### Mode 1: Using Command-line Flags

Run a single test with command-line flags:

```bash
# Download 1KB
go run ./cmd/client -host localhost:8080 -op download -size 1KB

# Upload 10MB
go run ./cmd/client -host localhost:8080 -op upload -size 10MB
```

### Mode 2: Using Configuration File

Create a `config.yaml` file (see `config.example.yaml`):

```yaml
hosts:
  - localhost:8080
  - localhost:8081
  - localhost:8082

tests:
  - name: "Small Upload Test"
    operation: upload
    repeat: 3
    size: 16KB
    timeout: 5s  # Optional: override default timeout

  - name: "Medium Download Test"
    operation: download
    repeat: 3
    size: 256KB
    # timeout not specified, will use default (10s)

  - name: "Large Upload Test"
    operation: upload
    repeat: 2
    size: 1MB
    timeout: 1m30s  # 1 minute 30 seconds
```

Run tests from config:

```bash
# Use default config.yaml in current directory
go run ./cmd/client

# Specify custom config file
go run ./cmd/client -config my-tests.yaml

# Combine config tests with additional flag-based test
go run ./cmd/client -config my-tests.yaml -host localhost:9000 -op download -size 512KB
```

### Client Flags

- `-config` - Path to configuration file (default: `config.yaml`)
- `-host` - Server address (host:port)
- `-op` - Operation type: `download` or `upload`
- `-size` - Data size with unit: `1KB`, `512KB`, `1MB`, `16MB`, etc.
- `-timeout` - Timeout duration for operations (default: `10s`, supports: `1s`, `500ms`, `1m`, `1m30s`, etc.)

### Configuration File Format

**YAML structure:**
```yaml
hosts:           # List of server addresses to test
  - host1:port1
  - host2:port2

tests:           # List of tests to execute
  - name: "Test Name"        # Descriptive name for the test
    operation: upload        # "upload" or "download"
    repeat: 3                # Number of times to repeat this test
    size: 16KB              # Data size (supports KB, MB, GB)
    timeout: 10s            # Timeout duration (optional, default: 10s)
                             # Supports: 1s, 500ms, 1m, 1m30s, etc.
```

### Execution Rules

1. **Config only**: If `config.yaml` exists and no flags specified → run tests from config
2. **Flags only**: If config doesn't exist but `-host`, `-op`, `-size` provided → run single test
3. **Config + Flags**: If both exist → run config tests PLUS additional test from flags
4. **Error cases**:
   - No config and no flags → error
   - `-config` specified but file doesn't exist → error

### Output Example

```
========== Starting Test Suite ==========
Hosts: [localhost:8080 localhost:8081]
Tests: 3
=========================================

--- Test: Small Upload Test on localhost:8080 ---
Attempt 1/3...
✓ Success: 15.2ms (1.03 MB/s)
Attempt 2/3...
✓ Success: 14.8ms (1.06 MB/s)
Attempt 3/3...
✓ Success: 15.1ms (1.04 MB/s)

--- Test: Small Upload Test on localhost:8081 ---
Attempt 1/3...
✓ Success: 16.3ms (0.96 MB/s)
...

========== Test Summary ==========
Total tests: 9
Successful: 9 (100.0%)
Failed: 0 (0.0%)
Average duration: 15.4ms
Average throughput: 1.02 MB/s
Total data transferred: 0.14 MB
==================================
```

## Examples

### Example 1: Single test with flags
```bash
# Terminal 1 - Start server
$ go run ./cmd/server
2026/01/04 12:00:00 Server started on port 8080
2026/01/04 12:00:00 Server started on port 8081
2026/01/04 12:00:00 Server started on port 8082
2026/01/04 12:00:00 Servers started on ports: [8080 8081 8082]
2026/01/04 12:00:00 Press Ctrl+C to stop

# Terminal 2 - Run single test
$ go run ./cmd/client -host localhost:8080 -op download -size 1MB
2026/01/04 12:00:05 Added test from flags: default test download 1MB on localhost:8080

========== Starting Test Suite ==========
Hosts: [localhost:8080]
Tests: 1
=========================================

--- Test: default test download 1MB on localhost:8080 ---
Attempt 1/1...
✓ Success: 15.2ms (65.79 MB/s)

========== Test Summary ==========
Total tests: 1
Successful: 1 (100.0%)
Failed: 0 (0.0%)
Average duration: 15.2ms
Average throughput: 65.79 MB/s
Total data transferred: 1.00 MB
==================================
```

### Example 2: Multiple tests from config
```bash
$ cp config.yaml.example config.yaml
$ go run ./cmd/client
2026/01/04 12:01:00 Loaded config from config.yaml: 5 tests, 3 hosts

========== Starting Test Suite ==========
Hosts: [localhost:8080 localhost:8081 localhost:8082]
Tests: 5
=========================================

--- Test: Small Upload Test on localhost:8080 ---
Attempt 1/3...
✓ Success: 8.3ms (1.89 MB/s)
Attempt 2/3...
✓ Success: 8.1ms (1.93 MB/s)
Attempt 3/3...
✓ Success: 8.2ms (1.91 MB/s)
...
```

### Example 3: Config + additional flag test
```bash
$ go run ./cmd/client -host localhost:9000 -op upload -size 2MB
2026/01/04 12:02:00 Loaded config from config.yaml: 5 tests, 3 hosts
2026/01/04 12:02:00 Added test from flags: default test upload 2MB on localhost:9000

========== Starting Test Suite ==========
Hosts: [localhost:8080 localhost:8081 localhost:8082 localhost:9000]
Tests: 6
=========================================
...
```

## Testing

### Quick test script:
```bash
# Start server in background
go run ./cmd/server &
SERVER_PID=$!

# Wait for server to start
sleep 1

# Run tests with flags
go run ./cmd/client -host localhost:8080 -op download -size 1KB
go run ./cmd/client -host localhost:8080 -op download -size 100KB
go run ./cmd/client -host localhost:8080 -op upload -size 1MB

# Or run from config
cp config.yaml.example config.yaml
go run ./cmd/client

# Stop server
kill $SERVER_PID
```

## Dependencies

- `gopkg.in/yaml.v3` - YAML parsing for configuration files

Install dependencies:
```bash
go mod download
```

## Limits

- Maximum data size: **16MB** per operation
- Port range: 1-65535
- Hash algorithm: SHA256
