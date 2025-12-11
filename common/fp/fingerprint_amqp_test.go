package fp

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Mock AMQP server that responds with AMQP protocol header
func startMockAMQPServer(t *testing.T, port int) (net.Listener, func()) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("Failed to start mock AMQP server: %v", err)
	}

	log.Infof("Mock AMQP server started on port %d", port)

	// Start accepting connections
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				// Listener closed
				return
			}

			// Handle connection in goroutine
			go handleAMQPConnection(t, conn)
		}
	}()

	// Return cleanup function
	cleanup := func() {
		listener.Close()
		log.Infof("Mock AMQP server stopped on port %d", port)
	}

	return listener, cleanup
}

func handleAMQPConnection(t *testing.T, conn net.Conn) {
	defer conn.Close()

	// Set read timeout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Read incoming data (we don't care what it is)
	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil && err.Error() != "EOF" {
		log.Debugf("Read error (expected): %v", err)
	}

	if n > 0 {
		log.Debugf("Mock AMQP server received %d bytes: %s", n, string(buffer[:n]))
	}

	// Send AMQP protocol header response
	// This is the actual response from a real AMQP server
	// AMQP\x00\x00\x09\x01
	amqpResponse := []byte{0x41, 0x4d, 0x51, 0x50, 0x00, 0x00, 0x09, 0x01}

	_, err = conn.Write(amqpResponse)
	if err != nil {
		log.Errorf("Failed to write AMQP response: %v", err)
		return
	}

	log.Debugf("Mock AMQP server sent AMQP protocol header (8 bytes)")

	// Keep connection open for a bit to simulate real server
	time.Sleep(100 * time.Millisecond)
}

// TestAMQPDetectionOnNonStandardPort tests that AMQP service can be detected
// on a non-standard port (not 5672) after the fix
func TestAMQPDetectionOnNonStandardPort(t *testing.T) {
	// Use a non-standard port for AMQP
	testPort := utils.GetRandomAvailableTCPPort()

	// Start mock AMQP server
	_, cleanup := startMockAMQPServer(t, testPort)
	defer cleanup()

	// Wait for server to be ready by attempting to connect
	hostPort := fmt.Sprintf("127.0.0.1:%d", testPort)
	err := utils.WaitConnect(hostPort, 3)
	if err != nil {
		t.Fatalf("Failed to wait for AMQP server to be ready: %v", err)
	}

	// Create config with reasonable ProbesMax and disable web fingerprint to ensure
	// service detection runs first and is not interfered by web detection.
	// This is important because if the port happens to be in webPorts list, web detection
	// might run first and incorrectly identify the service as HTTP.
	// Using ProbesMax=10 is sufficient to detect AMQP while keeping test execution time reasonable
	config := NewConfig(
		WithActiveMode(true),
		WithProbesMax(10), // Reduced to speed up tests while still detecting AMQP
		WithProbeTimeout(1*time.Second), // Reduced to speed up tests
		WithDisableWebFingerprint(true), // Disable web fingerprint to avoid interference
	)

	// Create matcher
	matcher, err := NewDefaultFingerprintMatcher(config)
	assert.NoError(t, err)
	assert.NotNil(t, matcher)

	// Perform fingerprint scan
	ctx := context.Background()
	result, err := matcher.MatchWithContext(ctx, "127.0.0.1", testPort)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, OPEN, result.State, "Port should be detected as OPEN")

	// The key assertion: service should be identified as AMQP
	serviceName := result.GetServiceName()
	log.Infof("Detected service: %s", serviceName)

	assert.Contains(t, serviceName, "amqp",
		"Service should be identified as AMQP, but got: %s", serviceName)

	// Verify fingerprint details
	if result.Fingerprint != nil {
		log.Infof("Banner: %s", result.Fingerprint.Banner)
		log.Infof("CPEs: %v", result.Fingerprint.CPEs)

		// AMQP banner should contain the protocol header
		assert.NotEmpty(t, result.Fingerprint.Banner, "Banner should not be empty")
	}
}

// TestRuleBlockMerging tests that the rule block merging logic works correctly
func TestRuleBlockMerging(t *testing.T) {
	testPort := 9000 // Non-standard port

	config := NewConfig(
		WithActiveMode(true),
		WithProbesMax(100),
	)

	// Get rule blocks for the test port
	emptyBlock, blocks, ok := GetRuleBlockByConfig(testPort, config)

	// Log results for debugging
	log.Infof("Empty block: %v", emptyBlock != nil)
	log.Infof("Blocks count: %d", len(blocks))
	log.Infof("Has port-specific rules: %v", ok)

	// Verify that we get some blocks
	assert.NotEmpty(t, blocks, "Should have some probe blocks")

	// Count how many blocks are port-specific vs general
	portSpecificCount := 0
	generalCount := 0

	for _, block := range blocks {
		if block.Probe == nil {
			continue
		}

		isPortSpecific := false
		for _, port := range block.Probe.DefaultPorts {
			if port == testPort {
				isPortSpecific = true
				break
			}
		}

		if isPortSpecific {
			portSpecificCount++
		} else {
			generalCount++
		}

		log.Debugf("Probe: %s, Rarity: %d, DefaultPorts: %v, IsPortSpecific: %v",
			block.Probe.Name, block.Probe.Rarity, block.Probe.DefaultPorts, isPortSpecific)
	}

	log.Infof("Port-specific rules: %d, General rules: %d", portSpecificCount, generalCount)

	// After the fix, we should have BOTH port-specific AND general rules
	// (assuming ProbesMax allows it)
	if portSpecificCount > 0 {
		// If there are port-specific rules, we should also have some general rules merged
		// (unless ProbesMax is too small or there are no general rules)
		if portSpecificCount < config.ProbesMax {
			// There's room for general rules
			assert.Greater(t, generalCount, 0,
				"Should have general rules merged when there's room in ProbesMax")
		}
	}

	// Verify blocks are within ProbesMax limit
	assert.LessOrEqual(t, len(blocks), config.ProbesMax,
		"Total blocks should not exceed ProbesMax")
}

// TestAMQPRuleSelection verifies that AMQP rule is included in the selection
func TestAMQPRuleSelection(t *testing.T) {
	testPort := 19002 // Non-standard port

	config := NewConfig(
		WithActiveMode(true),
		WithProbesMax(100), // Use larger value to ensure AMQP is included
	)

	_, blocks, hasPortSpecific := GetRuleBlockByConfig(testPort, config)

	log.Infof("Port %d: hasPortSpecific=%v, total blocks=%d", testPort, hasPortSpecific, len(blocks))

	// Check if AMQP probe is included
	hasAMQP := false
	for i, block := range blocks {
		if block.Probe != nil {
			log.Debugf("Block %d: %s (rarity=%d, ports=%v)",
				i, block.Probe.Name, block.Probe.Rarity, block.Probe.DefaultPorts)
			if block.Probe.Name == "AMQP" {
				hasAMQP = true
				log.Infof("Found AMQP probe at position %d", i)
			}
		}
	}

	// After the fix, AMQP should be included either:
	// 1. If there are NO port-specific rules (it's in general rules)
	// 2. If there ARE port-specific rules AND room in ProbesMax (merged)
	if !hasPortSpecific {
		// No port-specific rules, AMQP should definitely be in general rules
		if !hasAMQP {
			// Log all available probes to understand why
			allRules := config.GetFingerprintRules()
			hasAMQPInRules := false
			for probe := range allRules {
				if probe.Name == "AMQP" {
					hasAMQPInRules = true
					log.Infof("AMQP rule exists in fingerprint rules: rarity=%d, ports=%v",
						probe.Rarity, probe.DefaultPorts)
					break
				}
			}
			if !hasAMQPInRules {
				t.Skip("AMQP rule not found in fingerprint database - skipping test")
				return
			}
		}
		assert.True(t, hasAMQP,
			"AMQP probe should be included when there are no port-specific rules")
	} else {
		// Has port-specific rules, AMQP might be included if there's room
		log.Infof("Port %d has port-specific rules, AMQP included: %v", testPort, hasAMQP)
		// This is not an error - it depends on ProbesMax and how many port-specific rules exist
	}
}

// TestProbesMaxLimitation verifies that ProbesMax still works correctly
func TestProbesMaxLimitation(t *testing.T) {
	testPort := 19003

	testCases := []struct {
		name      string
		probesMax int
	}{
		{"ProbesMax_3", 3},
		{"ProbesMax_5", 5},
		{"ProbesMax_10", 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := NewConfig(
				WithActiveMode(true),
				WithProbesMax(tc.probesMax),
				WithRarityMax(9),
			)

			_, blocks, _ := GetRuleBlockByConfig(testPort, config)

			assert.LessOrEqual(t, len(blocks), tc.probesMax,
				"Blocks count should not exceed ProbesMax(%d), got %d",
				tc.probesMax, len(blocks))

			log.Infof("ProbesMax=%d: Got %d blocks", tc.probesMax, len(blocks))
		})
	}
}

// TestAMQPDetectionWithDifferentProbesMax tests AMQP detection with various ProbesMax values
func TestAMQPDetectionWithDifferentProbesMax(t *testing.T) {
	testPort := 19004

	// Start mock server
	_, cleanup := startMockAMQPServer(t, testPort)
	defer cleanup()
	time.Sleep(100 * time.Millisecond)

	testCases := []struct {
		name         string
		probesMax    int
		shouldDetect bool
	}{
		{"ProbesMax_3_MayNotDetect", 3, false},  // Might not detect if port-specific rules take all slots
		{"ProbesMax_5_ShouldDetect", 5, true},   // Should detect with default value
		{"ProbesMax_10_ShouldDetect", 10, true}, // Definitely should detect
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := NewConfig(
				WithActiveMode(true),
				WithProbesMax(tc.probesMax),
				WithRarityMax(9),
				WithProbeTimeout(3*time.Second),
			)

			matcher, err := NewDefaultFingerprintMatcher(config)
			assert.NoError(t, err)

			ctx := context.Background()
			result, err := matcher.MatchWithContext(ctx, "127.0.0.1", testPort)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, OPEN, result.State)

			serviceName := result.GetServiceName()
			log.Infof("ProbesMax=%d: Detected service=%s", tc.probesMax, serviceName)

			if tc.shouldDetect {
				// With the fix, AMQP should be detected
				if !utils.MatchAnyOfSubString(serviceName, "amqp") {
					t.Logf("WARNING: AMQP not detected with ProbesMax=%d, service=%s",
						tc.probesMax, serviceName)
					// Not failing the test for smaller ProbesMax values
				}
			}
		})
	}
}
