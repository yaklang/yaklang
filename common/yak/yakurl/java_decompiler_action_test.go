package yakurl

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestJavaDecompilerAction_Get(t *testing.T) {
	// Skip if no JAR file available
	jarPath := os.Getenv("TEST_JAR_PATH")
	if jarPath == "" {
		t.Skip("TEST_JAR_PATH environment variable not set, skipping test")
	}

	// Check if the JAR file exists
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		t.Skipf("JAR file not found at %s, skipping test", jarPath)
	}

	action := newJavaDecompilerAction()

	// Test JAR directory listing
	t.Run("TestListJarRoot", func(t *testing.T) {
		url, err := CreateUrlFromString("javadec:///jar")
		if err != nil {
			t.Fatalf("Failed to create URL: %v", err)
		}

		// Add query parameters
		url.Query = append(url.Query, &ypb.KVPair{
			Key:   "jar",
			Value: jarPath,
		})

		params := &ypb.RequestYakURLParams{
			Url:    url,
			Method: "GET",
		}

		resp, err := action.Get(params)
		if err != nil {
			t.Fatalf("Failed to list JAR root: %v", err)
		}

		if resp.Total <= 0 {
			t.Errorf("Expected at least one resource in JAR, got %d", resp.Total)
		}

		// Check if resources have the correct structure
		for _, resource := range resp.Resources {
			if resource.ResourceName == "" {
				t.Errorf("Resource name should not be empty")
			}
			if resource.Url == nil {
				t.Errorf("Resource URL should not be nil")
			}
			if resource.ResourceType != "dir" && resource.ResourceType != "file" {
				t.Errorf("Unexpected resource type: %s", resource.ResourceType)
			}
		}
	})

	// Test listing a subdirectory inside JAR
	t.Run("TestListJarSubdirectory", func(t *testing.T) {
		// First find a directory from the root listing
		rootURL, _ := CreateUrlFromString("javadec:///jar")
		rootURL.Query = append(rootURL.Query, &ypb.KVPair{
			Key:   "jar",
			Value: jarPath,
		})

		rootParams := &ypb.RequestYakURLParams{
			Url:    rootURL,
			Method: "GET",
		}

		rootResp, err := action.Get(rootParams)
		if err != nil {
			t.Fatalf("Failed to list JAR root: %v", err)
		}

		var dirPath string
		for _, res := range rootResp.Resources {
			if res.ResourceType == "dir" {
				dirPath = res.Path
				break
			}
		}

		if dirPath == "" {
			t.Skip("No directories found in JAR root, skipping subdirectory test")
		}

		// List subdirectory
		url, _ := CreateUrlFromString("javadec:///jar")
		url.Query = append(url.Query,
			&ypb.KVPair{Key: "jar", Value: jarPath},
			&ypb.KVPair{Key: "dir", Value: dirPath},
		)

		params := &ypb.RequestYakURLParams{
			Url:    url,
			Method: "GET",
		}

		resp, err := action.Get(params)
		if err != nil {
			t.Fatalf("Failed to list JAR subdirectory: %v", err)
		}

		t.Logf("Found %d resources in subdirectory %s", resp.Total, dirPath)
	})

	// Test decompiling a class file
	t.Run("TestDecompileClass", func(t *testing.T) {
		// First find a .class file from the root listing (or recursively)
		var classPath string

		// Helper function to find a class file
		var findClassFile func(string) bool
		findClassFile = func(dirPath string) bool {
			url, _ := CreateUrlFromString("javadec:///jar")
			url.Query = append(url.Query,
				&ypb.KVPair{Key: "jar", Value: jarPath},
				&ypb.KVPair{Key: "dir", Value: dirPath},
			)

			params := &ypb.RequestYakURLParams{
				Url:    url,
				Method: "GET",
			}

			resp, err := action.Get(params)
			if err != nil {
				return false
			}

			for _, res := range resp.Resources {
				if strings.HasSuffix(res.ResourceName, ".class") {
					classPath = res.Path
					return true
				}

				if res.ResourceType == "dir" {
					if findClassFile(res.Path) {
						return true
					}
				}
			}

			return false
		}

		if !findClassFile(".") {
			t.Skip("No .class files found in JAR, skipping decompilation test")
		}

		// Decompile the class
		url, _ := CreateUrlFromString("javadec:///class")
		url.Query = append(url.Query,
			&ypb.KVPair{Key: "jar", Value: jarPath},
			&ypb.KVPair{Key: "class", Value: classPath},
		)

		params := &ypb.RequestYakURLParams{
			Url:    url,
			Method: "GET",
		}

		resp, err := action.Get(params)
		if err != nil {
			t.Fatalf("Failed to decompile class: %v", err)
		}

		if len(resp.Resources) != 1 {
			t.Errorf("Expected 1 resource, got %d", len(resp.Resources))
		}

		resource := resp.Resources[0]
		if resource.ResourceType != "class" {
			t.Errorf("Expected resource type 'class', got '%s'", resource.ResourceType)
		}

		// Check if the resource has the encoded content
		hasContent := false
		for _, extra := range resource.Extra {
			if extra.Key == "content" {
				hasContent = true
				if extra.Value == "" {
					t.Errorf("Content should not be empty")
				}
				break
			}
		}

		if !hasContent {
			t.Errorf("Resource should have 'content' field in Extra")
		}
	})

	// Test error cases
	t.Run("TestErrorCases", func(t *testing.T) {
		// Missing jar parameter
		url1, _ := CreateUrlFromString("javadec:///jar")
		params1 := &ypb.RequestYakURLParams{
			Url: url1,
		}

		_, err := action.Get(params1)
		if err == nil {
			t.Errorf("Expected error for missing jar parameter")
		}

		// Invalid jar path
		url2, _ := CreateUrlFromString("javadec:///jar")
		url2.Query = append(url2.Query, &ypb.KVPair{
			Key:   "jar",
			Value: "/path/to/nonexistent.jar",
		})

		params2 := &ypb.RequestYakURLParams{
			Url: url2,
		}

		_, err = action.Get(params2)
		if err == nil {
			t.Errorf("Expected error for invalid jar path")
		}

		// Missing class parameter
		url3, _ := CreateUrlFromString("javadec:///class")
		url3.Query = append(url3.Query, &ypb.KVPair{
			Key:   "jar",
			Value: jarPath,
		})

		params3 := &ypb.RequestYakURLParams{
			Url: url3,
		}

		_, err = action.Get(params3)
		if err == nil {
			t.Errorf("Expected error for missing class parameter")
		}

		// Invalid class path
		url4, _ := CreateUrlFromString("javadec:///class")
		url4.Query = append(url4.Query,
			&ypb.KVPair{Key: "jar", Value: jarPath},
			&ypb.KVPair{Key: "class", Value: "nonexistent.class"},
		)

		params4 := &ypb.RequestYakURLParams{
			Url: url4,
		}

		_, err = action.Get(params4)
		if err == nil {
			t.Errorf("Expected error for invalid class path")
		}

		// Unsupported path
		url5, _ := CreateUrlFromString("javadec://unsupported/path")
		params5 := &ypb.RequestYakURLParams{
			Url: url5,
		}

		_, err = action.Get(params5)
		if err == nil {
			t.Errorf("Expected error for unsupported path")
		}
	})
}
