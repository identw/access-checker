package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/identw/access-checker/pkg/protocol"
	"gopkg.in/yaml.v3"
)

// Config represents the YAML configuration structure
type Config struct {
	Hosts []string `yaml:"hosts"`
	Tests []Test   `yaml:"tests"`
}

// Test represents a single test configuration
type Test struct {
	Name      string `yaml:"name"`
	Operation string `yaml:"operation"`
	Repeat    int    `yaml:"repeat"`
	Size      string `yaml:"size"`
}

// TestResult holds the results of a test execution
type TestResult struct {
	TestName  string
	Host      string
	Attempt   int
	Success   bool
	Duration  time.Duration
	BytesSent uint32
	Error     error
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	host := flag.String("host", "", "Server address (host:port)")
	operation := flag.String("op", "", "Operation: 'download' or 'upload'")
	size := flag.String("size", "", "Data size (e.g., 1KB, 512KB, 1MB, 16MB)")
	flag.Parse()

	var tests []Test
	var hosts []string
	
	// Check if config file exists
	configExists := fileExists(*configPath)
	configSpecified := flag.Lookup("config").Value.String() != flag.Lookup("config").DefValue
	
	// Load config if it exists
	if configExists {
		config, err := loadConfig(*configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		tests = config.Tests
		hosts = config.Hosts
		log.Printf("Loaded config from %s: %d tests, %d hosts\n", *configPath, len(tests), len(hosts))
	} else if configSpecified {
		// Config was explicitly specified but doesn't exist
		log.Fatalf("Config file specified but not found: %s", *configPath)
	}
	
	// Check if flags are provided
	flagsProvided := *host != "" && *operation != "" && *size != ""
	
	// Add test from flags if provided
	if flagsProvided {
		flagTest := Test{
			Name:      fmt.Sprintf("default test %s %s", *operation, *size),
			Operation: *operation,
			Repeat:    1,
			Size:      *size,
		}
		tests = append(tests, flagTest)
		
		// Add host from flag if not already in hosts list
		if !contains(hosts, *host) {
			hosts = append(hosts, *host)
		}
		
		log.Printf("Added test from flags: %s on %s\n", flagTest.Name, *host)
	}
	
	// Validate that we have tests to run
	if len(tests) == 0 {
		log.Fatalf("No tests to run. Either provide a config file or specify --host, --op, and --size flags")
	}
	
	if len(hosts) == 0 {
		log.Fatalf("No hosts specified. Either provide hosts in config or use --host flag")
	}
	
	// Execute all tests
	log.Printf("\n========== Starting Test Suite ==========\n")
	log.Printf("Hosts: %v\n", hosts)
	log.Printf("Tests: %d\n", len(tests))
	log.Printf("=========================================\n\n")
	
	var allResults []TestResult
	
	for _, test := range tests {
		if err := validateTest(&test); err != nil {
			log.Printf("⚠ Skipping invalid test '%s': %v\n", test.Name, err)
			continue
		}
		
		for _, host := range hosts {
			results := executeTest(test, host)
			allResults = append(allResults, results...)
		}
	}
	
	// Print summary
	printSummary(allResults)
}

// executeTest runs a single test on a specific host
func executeTest(test Test, host string) []TestResult {
	log.Printf("\n--- Test: %s on %s ---\n", test.Name, host)
	
	dataSize, err := parseSize(test.Size)
	if err != nil {
		log.Printf("✗ Invalid size format '%s': %v\n", test.Size, err)
		return []TestResult{{
			TestName: test.Name,
			Host:     host,
			Success:  false,
			Error:    err,
		}}
	}
	
	var results []TestResult
	
	for i := 1; i <= test.Repeat; i++ {
		log.Printf("Attempt %d/%d...\n", i, test.Repeat)
		
		conn, err := net.Dial("tcp", host)
		if err != nil {
			log.Printf("✗ Failed to connect: %v\n", err)
			results = append(results, TestResult{
				TestName: test.Name,
				Host:     host,
				Attempt:  i,
				Success:  false,
				Error:    err,
			})
			continue
		}
		
		var testErr error
		var duration time.Duration
		
		switch test.Operation {
		case "download":
			duration, testErr = performDownload(conn, dataSize)
		case "upload":
			duration, testErr = performUpload(conn, dataSize)
		default:
			testErr = fmt.Errorf("unknown operation: %s", test.Operation)
		}
		
		conn.Close()
		
		result := TestResult{
			TestName:  test.Name,
			Host:      host,
			Attempt:   i,
			Success:   testErr == nil,
			Duration:  duration,
			BytesSent: dataSize,
			Error:     testErr,
		}
		
		if testErr != nil {
			log.Printf("✗ Failed: %v\n", testErr)
		} else {
			throughput := float64(dataSize) / duration.Seconds() / 1024 / 1024
			log.Printf("✓ Success: %v (%.2f MB/s)\n", duration, throughput)
		}
		
		results = append(results, result)
		
		// Small delay between attempts
		if i < test.Repeat {
			time.Sleep(100 * time.Millisecond)
		}
	}
	
	return results
}

// validateTest checks if test configuration is valid
func validateTest(test *Test) error {
	if test.Name == "" {
		return fmt.Errorf("test name is required")
	}
	if test.Operation != "download" && test.Operation != "upload" {
		return fmt.Errorf("operation must be 'download' or 'upload'")
	}
	if test.Repeat < 1 {
		test.Repeat = 1 // Default to 1 if not specified
	}
	if test.Size == "" {
		return fmt.Errorf("size is required")
	}
	
	// Validate size format
	_, err := parseSize(test.Size)
	if err != nil {
		return fmt.Errorf("invalid size format: %w", err)
	}
	
	return nil
}

// loadConfig reads and parses the YAML configuration file
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	
	return &config, nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// contains checks if a string slice contains a value
func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// printSummary displays test results summary
func printSummary(results []TestResult) {
	log.Printf("\n\n========== Test Summary ==========\n")
	
	totalTests := len(results)
	successCount := 0
	failCount := 0
	var totalDuration time.Duration
	var totalBytes uint32
	
	for _, result := range results {
		if result.Success {
			successCount++
			totalDuration += result.Duration
			totalBytes += result.BytesSent
		} else {
			failCount++
		}
	}
	
	log.Printf("Total tests: %d\n", totalTests)
	log.Printf("Successful: %d (%.1f%%)\n", successCount, float64(successCount)/float64(totalTests)*100)
	log.Printf("Failed: %d (%.1f%%)\n", failCount, float64(failCount)/float64(totalTests)*100)
	
	if successCount > 0 {
		avgDuration := totalDuration / time.Duration(successCount)
		avgThroughput := float64(totalBytes) / totalDuration.Seconds() / 1024 / 1024
		log.Printf("Average duration: %v\n", avgDuration)
		log.Printf("Average throughput: %.2f MB/s\n", avgThroughput)
		log.Printf("Total data transferred: %.2f MB\n", float64(totalBytes)/1024/1024)
	}
	
	log.Printf("==================================\n")
}

// performDownload requests data from server and validates hash
func performDownload(conn net.Conn, size uint32) (time.Duration, error) {
	start := time.Now()

	// Send download request: type + size
	n, err := conn.Write([]byte{protocol.MessageTypeDownload})
	if err != nil {
		return 0, fmt.Errorf("error writing message type: %w", err)
	}
	if n != 1 {
		return 0, fmt.Errorf("wrote %d bytes instead of 1 for message type", n)
	}

	err = binary.Write(conn, binary.BigEndian, size)
	if err != nil {
		return 0, fmt.Errorf("error writing size: %w", err)
	}

	reader := bufio.NewReader(conn)

	// Read response size
	var responseSize uint32
	err = binary.Read(reader, binary.BigEndian, &responseSize)
	if err != nil {
		return 0, fmt.Errorf("error reading response size: %w", err)
	}

	if responseSize != size {
		return 0, fmt.Errorf("unexpected response size: expected %d, got %d", size, responseSize)
	}

	// Read data
	data := make([]byte, responseSize)
	_, err = io.ReadFull(reader, data)
	if err != nil {
		return 0, fmt.Errorf("error reading data: %w", err)
	}

	// Read hash
	var receivedHash [32]byte
	_, err = io.ReadFull(reader, receivedHash[:])
	if err != nil {
		return 0, fmt.Errorf("error reading hash: %w", err)
	}

	duration := time.Since(start)

	// Validate hash
	calculatedHash := sha256.Sum256(data)
	if calculatedHash != receivedHash {
		return duration, fmt.Errorf("hash validation failed")
	}

	return duration, nil
}

// performUpload sends data to server for validation
func performUpload(conn net.Conn, size uint32) (time.Duration, error) {
	// Generate random data
	data := make([]byte, size)
	rand.Read(data)

	// Calculate hash
	hash := sha256.Sum256(data)

	start := time.Now()

	// Send upload request: type + size + data + hash
	n, err := conn.Write([]byte{protocol.MessageTypeUpload})
	if err != nil {
		return 0, fmt.Errorf("error writing message type: %w", err)
	}
	if n != 1 {
		return 0, fmt.Errorf("wrote %d bytes instead of 1 for message type", n)
	}

	err = binary.Write(conn, binary.BigEndian, size)
	if err != nil {
		return 0, fmt.Errorf("error writing size: %w", err)
	}

	n, err = conn.Write(data)
	if err != nil {
		return 0, fmt.Errorf("error writing data: %w", err)
	}
	if n != int(size) {
		return 0, fmt.Errorf("wrote %d bytes instead of %d for data", n, size)
	}

	n, err = conn.Write(hash[:])
	if err != nil {
		return 0, fmt.Errorf("error writing hash: %w", err)
	}
	if n != 32 {
		return 0, fmt.Errorf("wrote %d bytes instead of 32 for hash", n)
	}

	// Read result
	result := make([]byte, 1)
	_, err = io.ReadFull(conn, result)
	if err != nil {
		return 0, fmt.Errorf("error reading result: %w", err)
	}

	duration := time.Since(start)

	if result[0] != 1 {
		return duration, fmt.Errorf("server validation failed")
	}

	return duration, nil
}

// parseSize parses size strings like "1KB", "512KB", "1MB", "16MB"
func parseSize(sizeStr string) (uint32, error) {
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))

	multiplier := uint64(1)
	numStr := sizeStr

	if strings.HasSuffix(sizeStr, "KB") {
		multiplier = 1024
		numStr = strings.TrimSuffix(sizeStr, "KB")
	} else if strings.HasSuffix(sizeStr, "MB") {
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(sizeStr, "MB")
	} else if strings.HasSuffix(sizeStr, "GB") {
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(sizeStr, "GB")
	} else if strings.HasSuffix(sizeStr, "B") {
		multiplier = 1
		numStr = strings.TrimSuffix(sizeStr, "B")
	}

	num, err := strconv.ParseUint(numStr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %w", err)
	}

	result := num * multiplier
	if result > uint64(protocol.MaxDataSize) {
		return 0, fmt.Errorf("size exceeds maximum")
	}

	return uint32(result), nil
}
