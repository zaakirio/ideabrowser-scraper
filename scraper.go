package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	baseURL = "https://www.ideabrowser.com"
)

const version = "1.0.0"

var (
	// Configuration from environment
	anonKey      string
	projectURL   string
	email        string
	password     string

	// Command-line flags
	outputDir   string
	saveHTML    bool
	verbose     bool
	showHelp    bool
	showVersion bool
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	User         interface{} `json:"user,omitempty"`
}

// IdeaData represents the complete data structure for an idea
type IdeaData struct {
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Date        string            `json:"date"`
	Tags        []string          `json:"tags,omitempty"`
	FrameworkFit *FrameworkData   `json:"framework_fit,omitempty"`
	ACP         *ACPData          `json:"acp,omitempty"`
	BuildInfo   map[string]string `json:"build_info,omitempty"`
	FounderFit  map[string]string `json:"founder_fit,omitempty"`
	ValueLadder map[string]string `json:"value_ladder,omitempty"`
	WhyNow      map[string]string `json:"why_now,omitempty"`
	ProofSignals map[string]string `json:"proof_signals,omitempty"`
	MarketGap   map[string]string `json:"market_gap,omitempty"`
	ExecutionPlan map[string]string `json:"execution_plan,omitempty"`
	Metrics     map[string]interface{} `json:"metrics,omitempty"`
}

// FrameworkData represents the Framework Fit metrics
type FrameworkData struct {
	ValueEquation struct {
		Score       int    `json:"score"`
		Rating      string `json:"rating"`
		Description string `json:"description,omitempty"`
	} `json:"value_equation"`
	MarketMatrix struct {
		Position    string `json:"position"`
		Uniqueness  string `json:"uniqueness"`
		Value       string `json:"value"`
		Description string `json:"description,omitempty"`
	} `json:"market_matrix"`
	ACPFramework struct {
		Audience  int `json:"audience_score"`
		Community int `json:"community_score"`
		Product   int `json:"product_score"`
		Overall   int `json:"overall_score"`
	} `json:"acp_framework"`
	ValueLadderStages []string `json:"value_ladder_stages,omitempty"`
}

// ACPData represents Audience, Customer, Problem data
type ACPData struct {
	Audience struct {
		Description string   `json:"description"`
		Size        string   `json:"size"`
		Demographics map[string]string `json:"demographics,omitempty"`
	} `json:"audience"`
	Customer struct {
		Description string   `json:"description"`
		Segments    []string `json:"segments,omitempty"`
		Behaviors   []string `json:"behaviors,omitempty"`
	} `json:"customer"`
	Problem struct {
		Description string   `json:"description"`
		PainPoints  []string `json:"pain_points,omitempty"`
		CurrentSolutions []string `json:"current_solutions,omitempty"`
	} `json:"problem"`
}

// loginWithEmail authenticates using email and password
func loginWithEmail(email, password string) (*TokenResponse, int64, error) {
	data := map[string]string{
		"email":    email,
		"password": password,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequest("POST", projectURL+"/auth/v1/token?grant_type=password", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("apikey", anonKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("login failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, 0, err
	}

	expiresAt := time.Now().Unix() + int64(tokenResp.ExpiresIn)
	if verbose {
		fmt.Printf("Login successful (token expires at %s)\n", time.Unix(expiresAt, 0).Format(time.RFC3339))
	}

	return &tokenResp, expiresAt, nil
}

func refreshSupabaseToken(refreshToken string) (*TokenResponse, int64, error) {
	data := map[string]string{"refresh_token": refreshToken}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequest("POST", projectURL+"/auth/v1/token?grant_type=refresh_token", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("apikey", anonKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("failed to refresh token: status %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, 0, err
	}

	expiresAt := time.Now().Unix() + int64(tokenResp.ExpiresIn)
	if verbose {
		fmt.Printf("Token refreshed successfully (expires at %s)\n", time.Unix(expiresAt, 0).Format(time.RFC3339))
	}

	return &tokenResp, expiresAt, nil
}

func getIdeaSlug() (string, error) {
	url := baseURL + "/idea-of-the-day"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36 OPR/120.0.0.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Accept-Language", "en-GB,en-US;q=0.9,en;q=0.8,ru;q=0.7,ar;q=0.6,pl;q=0.5,de;q=0.4,fr;q=0.3,zh-CN;q=0.2,zh;q=0.1,th;q=0.1,vi;q=0.1")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch idea of the day: status %d", resp.StatusCode)
	}

	var reader io.ReadCloser
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return "", err
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	htmlContent := string(body)
	// Look for href containing "/idea/" and extract the full slug
	// The slug is everything between /idea/ and the next /
	re := regexp.MustCompile(`href="/idea/([^/]+)/`)
	matches := re.FindStringSubmatch(htmlContent)
	if len(matches) > 1 {
		slug := matches[1]
		if verbose {
			fmt.Printf("Extracted slug: %s\n", slug)
		}
		return slug, nil
	}
	
	// Try alternative pattern if first one doesn't match
	re2 := regexp.MustCompile(`/idea/([a-z0-9-]+)/`)
	matches2 := re2.FindStringSubmatch(htmlContent)
	if len(matches2) > 1 {
		slug := matches2[1]
		if verbose {
			fmt.Printf("Extracted slug (alternative pattern): %s\n", slug)
		}
		return slug, nil
	}

	return "", fmt.Errorf("could not find idea slug in HTML")
}

func scrapePage(url string, accessToken string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36 OPR/120.0.0.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Accept-Language", "en-GB,en-US;q=0.9,en;q=0.8,ru;q=0.7,ar;q=0.6,pl;q=0.5,de;q=0.4,fr;q=0.3,zh-CN;q=0.2,zh;q=0.1,th;q=0.1,vi;q=0.1")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Referer", "https://www.ideabrowser.com/idea-of-the-day")
	
	// Add authorization header for protected pages
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("unauthorized access to %s (token may be expired)", url)
		}
		return "", fmt.Errorf("failed to scrape %s: status %d", url, resp.StatusCode)
	}

	var reader io.ReadCloser
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return "", err
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func init() {
	flag.StringVar(&outputDir, "output", ".", "Output directory for scraped data")
	flag.BoolVar(&saveHTML, "save-html", false, "Save raw HTML files for debugging")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&showHelp, "help", false, "Show help message")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
}

func loadConfig() error {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("error loading .env file: %v", err)
		}
	}

	// Get configuration from environment
	anonKey = os.Getenv("SUPABASE_ANON_KEY")
	projectURL = os.Getenv("SUPABASE_PROJECT_URL")
	email = os.Getenv("IDEABROWSER_EMAIL")
	password = os.Getenv("IDEABROWSER_PASSWORD")

	// Validate required configuration
	if anonKey == "" {
		return fmt.Errorf("SUPABASE_ANON_KEY environment variable is required")
	}
	if projectURL == "" {
		return fmt.Errorf("SUPABASE_PROJECT_URL environment variable is required")
	}
	if email == "" || password == "" {
		return fmt.Errorf("IDEABROWSER_EMAIL and IDEABROWSER_PASSWORD environment variables are required")
	}

	return nil
}

func printHelp() {
	fmt.Printf("IdeaBrowser Scraper v%s\n\n", version)
	fmt.Println("A tool for scraping business ideas from IdeaBrowser.com")
	fmt.Println("\nUsage:")
	fmt.Println("  ideabrowser-scraper [options]")
	fmt.Println("\nOptions:")
	flag.PrintDefaults()
	fmt.Println("\nExamples:")
	fmt.Println("  # Scrape today's idea")
	fmt.Println("  ideabrowser-scraper")
	fmt.Println("\n  # Scrape with verbose output")
	fmt.Println("  ideabrowser-scraper -verbose")
	fmt.Println("\n  # Save output to specific directory")
	fmt.Println("  ideabrowser-scraper -output ./ideas")
	fmt.Println("\nNote: Ensure you have set IDEABROWSER_EMAIL and IDEABROWSER_PASSWORD in your .env file")
}

func main() {
	flag.Parse()

	if showHelp {
		printHelp()
		return
	}

	if showVersion {
		fmt.Printf("IdeaBrowser Scraper v%s\n", version)
		return
	}

	// Load configuration
	if err := loadConfig(); err != nil {
		log.Fatalf("Configuration error: %v\n\nPlease ensure you have set up your .env file correctly.\nSee README.md for instructions.\n", err)
	}

	// Create output directory if it doesn't exist
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}

	// Setup authentication
	refreshTokenFile := filepath.Join(outputDir, "refresh_token.txt")
	var currentRefreshToken string
	var tokenResp *TokenResponse
	var expiresAt int64
	
	// Try to use saved refresh token first
	if data, err := os.ReadFile(refreshTokenFile); err == nil && len(data) > 0 {
		currentRefreshToken = strings.TrimSpace(string(data))
		if verbose {
			log.Printf("Found saved refresh token")
		}
	} else {
		// Login with email/password to get initial token
		log.Println("Authenticating with email/password...")
		var err error
		tokenResp, expiresAt, err = loginWithEmail(email, password)
		if err != nil {
			log.Fatalf("Authentication failed: %v\n", err)
		}
		currentRefreshToken = tokenResp.RefreshToken
		
		// Save refresh token for future use
		if err := os.WriteFile(refreshTokenFile, []byte(currentRefreshToken), 0644); err != nil {
			if verbose {
				fmt.Printf("Warning: failed to save refresh_token: %v\n", err)
			}
		}
	}

	// Get today's idea slug from public page
	log.Println("Fetching today's idea...")
	slug, err := getIdeaSlug()
	if err != nil {
		log.Fatalf("Failed to get today's idea: %v\n", err)
	}
	log.Printf("Found today's idea: %s\n", slug)

	// Define pages to scrape based on actual available sections
	pageURLs := []string{
		"/idea-of-the-day",                            // Page 1: Public main page
		path.Join("/idea", slug, "acp"),                // Page 2: ACP Framework (requires auth)
		path.Join("/idea", slug, "value-equation"),     // Page 3: Value Equation
		path.Join("/idea", slug, "value-matrix"),       // Page 4: Market Matrix  
		path.Join("/idea", slug, "value-ladder"),       // Page 5: Value ladder
		path.Join("/idea", slug, "build/landing-page"), // Page 6: Build landing page
		path.Join("/idea", slug, "founder-fit"),        // Page 7: Founder fit
		path.Join("/idea", slug, "why-now"),            // Page 8: Why now
		path.Join("/idea", slug, "proof-signals"),      // Page 9: Proof signals
		path.Join("/idea", slug, "market-gap"),         // Page 10: Market gap
		path.Join("/idea", slug, "execution-plan"),     // Page 11: Execution plan
	}

	// Store scraped pages for processing
	scrapedPages := make(map[string]string)

	log.Printf("Starting to scrape %d pages...\n", len(pageURLs))

	for i, pagePath := range pageURLs {
		fullURL := baseURL + pagePath
		if verbose {
			fmt.Printf("[%d/%d] Scraping: %s\n", i+1, len(pageURLs), pagePath)
		}

		// Page 1 is public; no token needed
		if i == 0 {
			content, err := scrapePage(fullURL, "")
			if err != nil {
				fmt.Printf("Failed to scrape page %d: %v\n", i+1, err)
				continue
			}
			if verbose {
				fmt.Printf("✓ Page %d scraped successfully (%d bytes)\n", i+1, len(content))
			}
			if saveHTML {
				htmlFile := filepath.Join(outputDir, fmt.Sprintf("page_%d.html", i+1))
				os.WriteFile(htmlFile, []byte(content), 0644)
			}
			scrapedPages[pagePath] = content
			continue
		}

		// Protected pages require authentication
		if tokenResp == nil || time.Now().Unix() >= expiresAt {
			if verbose {
				log.Println("Refreshing authentication token...")
			}
			var err error
			tokenResp, expiresAt, err = refreshSupabaseToken(currentRefreshToken)
			if err != nil {
				log.Fatalf("Token refresh failed: %v\n", err)
			}
			currentRefreshToken = tokenResp.RefreshToken
			
			// Save updated refresh token
			if err := os.WriteFile(refreshTokenFile, []byte(currentRefreshToken), 0644); err != nil {
				if verbose {
					fmt.Printf("Warning: failed to save refresh_token: %v\n", err)
				}
			}
		}

		content, err := scrapePage(fullURL, tokenResp.AccessToken)
		if err != nil {
			fmt.Printf("Failed to scrape page %d: %v\n", i+1, err)
			continue
		}
		if verbose {
			fmt.Printf("✓ Page %d scraped successfully (%d bytes)\n", i+1, len(content))
		}
		if saveHTML {
			htmlFile := filepath.Join(outputDir, fmt.Sprintf("page_%d.html", i+1))
			os.WriteFile(htmlFile, []byte(content), 0644)
		}
		
		// Store page content with simplified key
		pageKey := strings.TrimPrefix(pagePath, path.Join("/idea", slug) + "/")
		scrapedPages[pageKey] = content

		// Add delay to avoid rate limits
		time.Sleep(time.Second)
	}
	
	// Parse and save data to JSON
	log.Println("Parsing scraped data...")
	if err := parseAndSaveData(slug, scrapedPages, outputDir); err != nil {
		log.Fatalf("Failed to parse and save data: %v\n", err)
	}

	log.Println("✓ Scraping completed successfully!")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractTextBetween extracts text between two strings
func extractTextBetween(html, start, end string) string {
	startIdx := strings.Index(html, start)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(start)
	
	endIdx := strings.Index(html[startIdx:], end)
	if endIdx == -1 {
		return ""
	}
	
	return strings.TrimSpace(html[startIdx : startIdx+endIdx])
}

// cleanHTMLText removes HTML tags and cleans up text
func cleanHTMLText(text string) string {
	// Remove script and style elements
	re := regexp.MustCompile(`(?i)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>`)
	text = re.ReplaceAllString(text, "")
	
	// Remove HTML tags
	re = regexp.MustCompile(`<[^>]+>`)
	text = re.ReplaceAllString(text, " ")
	
	// Decode HTML entities
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&#x27;", "'")
	
	// Clean up whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	
	return strings.TrimSpace(text)
}

// extractIdeaInfo extracts the main idea information from the HTML
func extractIdeaInfo(html string) (string, string, string) {
	var title, description, date string
	
	// Extract title - look for the actual idea title
	// First try to find PicklePals or similar pattern
	titleRe := regexp.MustCompile(`(?s)<h1[^>]*>([^<]+(?:PicklePals|picklematch)[^<]*)</h1>`)
	if matches := titleRe.FindStringSubmatch(html); len(matches) > 1 {
		title = cleanHTMLText(matches[1])
	}
	
	// Fallback to other h1 patterns
	if title == "" {
		h1Re := regexp.MustCompile(`(?s)<h1[^>]*tracking-tight[^>]*>([^<]+)</h1>`)
		if matches := h1Re.FindStringSubmatch(html); len(matches) > 1 {
			title = cleanHTMLText(matches[1])
		}
	}
	
	// Extract description - look for the main description paragraph
	// Try to find the paragraph with class containing "text-lg text-gray-600"
	descRe := regexp.MustCompile(`(?s)<p[^>]*text-lg text-gray-600[^>]*>([^<]+)</p>`)
	if matches := descRe.FindStringSubmatch(html); len(matches) > 1 {
		description = cleanHTMLText(matches[1])
		// Decode HTML entities in description
		description = strings.ReplaceAll(description, "&quot;", "\"")
		description = strings.ReplaceAll(description, "&#x27;", "'")
	}
	
	// Extract date - look for the date pattern
	dateRe := regexp.MustCompile(`(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{1,2},\s+\d{4}`)
	if match := dateRe.FindString(html); match != "" {
		date = match
	}
	
	return title, description, date
}

// extractTags extracts tags/badges from the HTML
func extractTags(html string) []string {
	tags := []string{}
	tagSet := make(map[string]bool)
	
	// Look for badge/pill elements
	re := regexp.MustCompile(`<div[^>]*rounded-full[^>]*>.*?<span[^>]*>([^<]+)</span>`)
	matches := re.FindAllStringSubmatch(html, -1)
	
	for _, match := range matches {
		if len(match) > 1 {
			tag := cleanHTMLText(match[1])
			if tag != "" && !tagSet[tag] && len(tag) < 50 {
				tagSet[tag] = true
				tags = append(tags, tag)
			}
		}
	}
	
	return tags
}

// extractACPData extracts Audience, Customer, Problem data from ACP page
func extractACPData(html string) *ACPData {
	acp := &ACPData{}
	
	// Check if this is the ACP Framework page
	if !strings.Contains(html, "ACP Framework Analysis") {
		return acp
	}
	
	// Extract AUDIENCE ANALYSIS section
	audienceSection := extractTextBetween(html, "AUDIENCE ANALYSIS", "COMMUNITY ANALYSIS")
	if audienceSection != "" {
		// Extract Demographics
		if demo := extractTextBetween(audienceSection, "Demographics</p>", "</p>"); demo != "" {
			if acp.Audience.Demographics == nil {
				acp.Audience.Demographics = make(map[string]string)
			}
			acp.Audience.Demographics["primary"] = cleanHTMLText(demo)
		}
		
		// Extract Psychographics
		if psycho := extractTextBetween(audienceSection, "Psychographics</p>", "</p>"); psycho != "" {
			if acp.Audience.Demographics == nil {
				acp.Audience.Demographics = make(map[string]string)
			}
			acp.Audience.Demographics["psychographics"] = cleanHTMLText(psycho)
		}
		
		// Extract Platforms
		if platforms := extractTextBetween(audienceSection, "Platforms</p>", "</p>"); platforms != "" {
			if acp.Audience.Demographics == nil {
				acp.Audience.Demographics = make(map[string]string)
			}
			acp.Audience.Demographics["platforms"] = cleanHTMLText(platforms)
		}
		
		// Extract Unmet Needs
		if needs := extractTextBetween(audienceSection, "Unmet Needs</p>", "</p>"); needs != "" {
			acp.Audience.Description = cleanHTMLText(needs)
		}
		
		// Extract Content Gaps
		if gaps := extractTextBetween(audienceSection, "Content Gaps</p>", "</p>"); gaps != "" {
			if acp.Audience.Demographics == nil {
				acp.Audience.Demographics = make(map[string]string)
			}
			acp.Audience.Demographics["content_gaps"] = cleanHTMLText(gaps)
		}
		
		// Extract Differentiation
		if diff := extractTextBetween(audienceSection, "Differentiation</p>", "</p>"); diff != "" {
			if acp.Audience.Demographics == nil {
				acp.Audience.Demographics = make(map[string]string)
			}
			acp.Audience.Demographics["differentiation"] = cleanHTMLText(diff)
		}
		
		// Extract Secret Sauce
		if sauce := extractTextBetween(audienceSection, "Secret Sauce</p>", "</p>"); sauce != "" {
			if acp.Audience.Demographics == nil {
				acp.Audience.Demographics = make(map[string]string)
			}
			acp.Audience.Demographics["secret_sauce"] = cleanHTMLText(sauce)
		}
		
		// Extract Key Topics
		if topics := extractTextBetween(audienceSection, "Key Topics</p>", "</p>"); topics != "" {
			if acp.Audience.Demographics == nil {
				acp.Audience.Demographics = make(map[string]string)
			}
			acp.Audience.Demographics["key_topics"] = cleanHTMLText(topics)
		}
		
		// Extract Content Formats
		if formats := extractTextBetween(audienceSection, "Content Formats</p>", "</p>"); formats != "" {
			if acp.Audience.Demographics == nil {
				acp.Audience.Demographics = make(map[string]string)
			}
			acp.Audience.Demographics["content_formats"] = cleanHTMLText(formats)
		}
	}
	
	// Extract COMMUNITY ANALYSIS section
	communitySection := extractTextBetween(html, "COMMUNITY ANALYSIS", "PRODUCT ANALYSIS")
	if communitySection != "" {
		// Extract Primary Platform
		if platform := extractTextBetween(communitySection, "Primary Platform</p>", "</p>"); platform != "" {
			acp.Customer.Segments = append(acp.Customer.Segments, "Primary Platform: " + cleanHTMLText(platform))
		}
		
		// Extract Platform Rationale
		if rationale := extractTextBetween(communitySection, "Platform Rationale</p>", "</p>"); rationale != "" {
			acp.Customer.Segments = append(acp.Customer.Segments, "Platform Rationale: " + cleanHTMLText(rationale))
		}
		
		// Extract Secondary Platforms
		if secondary := extractTextBetween(communitySection, "Secondary Platforms</p>", "</p>"); secondary != "" {
			acp.Customer.Segments = append(acp.Customer.Segments, "Secondary Platforms: " + cleanHTMLText(secondary))
		}
		
		// Extract UGC Strategy
		if ugc := extractTextBetween(communitySection, "UGC Strategy</p>", "</p>"); ugc != "" {
			acp.Customer.Behaviors = append(acp.Customer.Behaviors, "UGC: " + cleanHTMLText(ugc))
		}
		
		// Extract Moderation Approach
		if moderation := extractTextBetween(communitySection, "Moderation Approach</p>", "</p>"); moderation != "" {
			acp.Customer.Behaviors = append(acp.Customer.Behaviors, "Moderation: " + cleanHTMLText(moderation))
		}
		
		// Extract Transparency
		if transparency := extractTextBetween(communitySection, "Transparency</p>", "</p>"); transparency != "" {
			acp.Customer.Behaviors = append(acp.Customer.Behaviors, "Transparency: " + cleanHTMLText(transparency))
		}
		
		// Extract Community Rituals
		if rituals := extractTextBetween(communitySection, "Community Rituals</p>", "</p>"); rituals != "" {
			acp.Customer.Behaviors = append(acp.Customer.Behaviors, "Rituals: " + cleanHTMLText(rituals))
		}
		
		// Extract Content Calendar
		if calendar := extractTextBetween(communitySection, "Content Calendar</p>", "</p>"); calendar != "" {
			acp.Customer.Behaviors = append(acp.Customer.Behaviors, "Calendar: " + cleanHTMLText(calendar))
		}
		
		// Extract Interaction Methods
		if interaction := extractTextBetween(communitySection, "Interaction Methods</p>", "</p>"); interaction != "" {
			acp.Customer.Behaviors = append(acp.Customer.Behaviors, "Interaction: " + cleanHTMLText(interaction))
		}
		
		// Set Customer Description
		if len(acp.Customer.Segments) > 0 || len(acp.Customer.Behaviors) > 0 {
			acp.Customer.Description = "Community-focused platform strategy with emphasis on engagement and trust building"
		}
	}
	
	// Extract PRODUCT ANALYSIS section
	productSection := extractTextBetween(html, "PRODUCT ANALYSIS", "EXECUTION PLAN")
	if productSection != "" {
		// Extract Core Offering Description
		if desc := extractTextBetween(productSection, "Description</p>", "</p>"); desc != "" {
			acp.Problem.Description = cleanHTMLText(desc)
		}
		
		// Extract Key Features
		if features := extractTextBetween(productSection, "Key Features</p>", "</p>"); features != "" {
			acp.Problem.PainPoints = append(acp.Problem.PainPoints, "Features: " + cleanHTMLText(features))
		}
		
		// Extract Value Proposition
		if value := extractTextBetween(productSection, "Value Proposition</p>", "</p>"); value != "" {
			acp.Problem.PainPoints = append(acp.Problem.PainPoints, "Value: " + cleanHTMLText(value))
		}
		
		// Extract MVP
		if mvp := extractTextBetween(productSection, "MVP</p>", "</p>"); mvp != "" {
			acp.Problem.CurrentSolutions = append(acp.Problem.CurrentSolutions, "MVP: " + cleanHTMLText(mvp))
		}
		
		// Extract Future Iterations
		if future := extractTextBetween(productSection, "Future Iterations</p>", "</p>"); future != "" {
			acp.Problem.CurrentSolutions = append(acp.Problem.CurrentSolutions, "Future: " + cleanHTMLText(future))
		}
		
		// Extract Community Integration
		if integration := extractTextBetween(productSection, "Community Integration</p>", "</p>"); integration != "" {
			acp.Problem.CurrentSolutions = append(acp.Problem.CurrentSolutions, "Integration: " + cleanHTMLText(integration))
		}
		
		// Extract Network Effects
		if network := extractTextBetween(productSection, "Network Effects</p>", "</p>"); network != "" {
			acp.Problem.PainPoints = append(acp.Problem.PainPoints, "Network Effects: " + cleanHTMLText(network))
		}
		
		// Extract Sticky Features
		if sticky := extractTextBetween(productSection, "Sticky Features</p>", "</p>"); sticky != "" {
			acp.Problem.PainPoints = append(acp.Problem.PainPoints, "Sticky Features: " + cleanHTMLText(sticky))
		}
		
		// Extract Usage Frequency
		if usage := extractTextBetween(productSection, "Usage Frequency</p>", "</p>"); usage != "" {
			acp.Problem.PainPoints = append(acp.Problem.PainPoints, "Usage: " + cleanHTMLText(usage))
		}
	}
	
	// Extract EXECUTION PLAN section
	executionSection := extractTextBetween(html, "EXECUTION PLAN", "</div></div></div>")
	if executionSection != "" {
		// Extract 90-Day Plan
		if plan := extractTextBetween(executionSection, "90-Day Plan</p>", "</p>"); plan != "" {
			if acp.Audience.Size == "" {
				acp.Audience.Size = "90-Day Plan: " + cleanHTMLText(plan)
			}
		}
	}
	
	return acp
}

// extractFrameworkData extracts Framework Fit metrics from the HTML
func extractFrameworkData(html string) *FrameworkData {
	framework := &FrameworkData{}
	
	// Check if this is the Value Equation page
	if strings.Contains(html, "Value Equation Analysis") {
		// Extract overall rating
		overallRe := regexp.MustCompile(`Overall Rating</p>.*?<div[^>]+>(\d+)</div>`)
		if matches := overallRe.FindStringSubmatch(html); len(matches) > 1 {
			if score, err := strconv.Atoi(matches[1]); err == nil {
				framework.ValueEquation.Score = score
			}
		}
		
		// Extract individual component scores and descriptions
		type ComponentData struct {
			Score string
			Description string
		}
		
		components := make(map[string]ComponentData)
		
		// Extract Dream Outcome
		dreamRe := regexp.MustCompile(`(?s)Dream Outcome</h1>.*?(\d+)<!-- -->/10</div>.*?<p[^>]*text-gray-600[^>]*>([^<]+)</p>`)
		if matches := dreamRe.FindStringSubmatch(html); len(matches) > 2 {
			components["Dream Outcome"] = ComponentData{
				Score: matches[1],
				Description: cleanHTMLText(matches[2]),
			}
		}
		
		// Extract Perceived Likelihood
		likelihoodRe := regexp.MustCompile(`(?s)Perceived Likelihood</h1>.*?(\d+)<!-- -->/10</div>.*?<p[^>]*text-gray-600[^>]*>([^<]+)</p>`)
		if matches := likelihoodRe.FindStringSubmatch(html); len(matches) > 2 {
			components["Perceived Likelihood"] = ComponentData{
				Score: matches[1],
				Description: cleanHTMLText(matches[2]),
			}
		}
		
		// Extract Time Delay
		timeRe := regexp.MustCompile(`(?s)Time Delay</h1>.*?(\d+)<!-- -->/10</div>.*?<p[^>]*text-gray-600[^>]*>([^<]+)</p>`)
		if matches := timeRe.FindStringSubmatch(html); len(matches) > 2 {
			components["Time Delay"] = ComponentData{
				Score: matches[1],
				Description: cleanHTMLText(matches[2]),
			}
		}
		
		// Extract Effort & Sacrifice
		effortRe := regexp.MustCompile(`(?s)Effort &amp; Sacrifice</h1>.*?(\d+)<!-- -->/10</div>.*?<p[^>]*text-gray-600[^>]*>([^<]+)</p>`)
		if matches := effortRe.FindStringSubmatch(html); len(matches) > 2 {
			components["Effort"] = ComponentData{
				Score: matches[1],
				Description: cleanHTMLText(matches[2]),
			}
		}
		
		// Set rating based on score
		if framework.ValueEquation.Score >= 8 {
			framework.ValueEquation.Rating = "Excellent"
		} else if framework.ValueEquation.Score >= 6 {
			framework.ValueEquation.Rating = "Good"
		} else if framework.ValueEquation.Score >= 4 {
			framework.ValueEquation.Rating = "Fair"
		} else {
			framework.ValueEquation.Rating = "Poor"
		}
		
		// Store component scores
		dreamScore := ""
		likelihoodScore := ""
		timeScore := ""
		effortScore := ""
		
		if data, ok := components["Dream Outcome"]; ok {
			dreamScore = data.Score
		}
		if data, ok := components["Perceived Likelihood"]; ok {
			likelihoodScore = data.Score
		}
		if data, ok := components["Time Delay"]; ok {
			timeScore = data.Score
		}
		if data, ok := components["Effort"]; ok {
			effortScore = data.Score
		}
		
		desc := fmt.Sprintf("Dream: %s/10, Likelihood: %s/10, Time: %s/10, Effort: %s/10",
			dreamScore, likelihoodScore, timeScore, effortScore)
		
		// Add detailed descriptions
		fullDesc := desc
		if data, ok := components["Dream Outcome"]; ok && data.Description != "" {
			fullDesc += fmt.Sprintf("\n\nDream Outcome: %s", data.Description)
		}
		if data, ok := components["Perceived Likelihood"]; ok && data.Description != "" {
			fullDesc += fmt.Sprintf("\n\nPerceived Likelihood: %s", data.Description)
		}
		if data, ok := components["Time Delay"]; ok && data.Description != "" {
			fullDesc += fmt.Sprintf("\n\nTime Delay: %s", data.Description)
		}
		if data, ok := components["Effort"]; ok && data.Description != "" {
			fullDesc += fmt.Sprintf("\n\nEffort & Sacrifice: %s", data.Description)
		}
		
		framework.ValueEquation.Description = fullDesc
	}
	
	// Check if this is the Market Matrix page
	if strings.Contains(html, "Market Matrix Analysis") {
		// Extract Uniqueness score
		uniqueRe := regexp.MustCompile(`Uniqueness</p>.*?(\d+)<!-- -->/10`)
		if matches := uniqueRe.FindStringSubmatch(html); len(matches) > 1 {
			if score, err := strconv.Atoi(matches[1]); err == nil {
				framework.MarketMatrix.Uniqueness = fmt.Sprintf("%d/10", score)
			}
		}
		
		// Extract Value score
		valueRe := regexp.MustCompile(`Value</p>.*?(\d+)<!-- -->/10`)
		if matches := valueRe.FindStringSubmatch(html); len(matches) > 1 {
			if score, err := strconv.Atoi(matches[1]); err == nil {
				framework.MarketMatrix.Value = fmt.Sprintf("%d/10", score)
			}
		}
		
		// Extract main analysis paragraph
		analysisRe := regexp.MustCompile(`(?s)Market Matrix Analysis</h1>.*?<p[^>]*text-gray-600[^>]*>([^<]+)</p>`)
		if matches := analysisRe.FindStringSubmatch(html); len(matches) > 1 {
			framework.MarketMatrix.Description = cleanHTMLText(matches[1])
		}
		
		// Extract position - look for highlighted quadrant with bg-yellow-50
		positionRe := regexp.MustCompile(`bg-yellow-50[^>]*>(?:[^>]*>)*[^>]*>([^<]+)</h3>`)
		if matches := positionRe.FindStringSubmatch(html); len(matches) > 1 {
			framework.MarketMatrix.Position = cleanHTMLText(matches[1])
		}
		
		// If position not found, look for the position in the badge
		if framework.MarketMatrix.Position == "" {
			// Look for the position badge near the bottom
			badgeSection := extractTextBetween(html, "Position Analysis", "Understanding the Quadrants")
			if badgeSection != "" {
				badgeRe := regexp.MustCompile(`text-amber-700">([^<]+)</span>`)
				if matches := badgeRe.FindStringSubmatch(badgeSection); len(matches) > 1 {
					position := cleanHTMLText(matches[1])
					if strings.Contains(position, "Tech Novelty") || strings.Contains(position, "Category King") ||
					   strings.Contains(position, "Low Impact") || strings.Contains(position, "Commodity Play") {
						framework.MarketMatrix.Position = position
					}
				}
			}
		}
		
		// Extract position explanation
		positionExplainRe := regexp.MustCompile(framework.MarketMatrix.Position + `</span>.*?<p[^>]*>([^<]+)</p>`)
		if matches := positionExplainRe.FindStringSubmatch(html); len(matches) > 1 {
			if framework.MarketMatrix.Description != "" {
				framework.MarketMatrix.Description += "\n\nPosition Analysis: " + cleanHTMLText(matches[1])
			}
		}
		
		// Extract quadrant descriptions for context
		quadrantSection := extractTextBetween(html, "Understanding the Quadrants", "</div></div></div>")
		if quadrantSection != "" {
			quadrantDetails := ""
			
			// Category King
			if catKing := extractTextBetween(quadrantSection, "Category King</h1>", "</p>"); catKing != "" {
				quadrantDetails += "\n\nCategory King: " + cleanHTMLText(catKing)
			}
			
			// Tech Novelty
			if techNov := extractTextBetween(quadrantSection, "Tech Novelty</h1>", "</p>"); techNov != "" {
				quadrantDetails += "\n\nTech Novelty: " + cleanHTMLText(techNov)
			}
			
			// Commodity Play
			if commodity := extractTextBetween(quadrantSection, "Commodity Play</h1>", "</p>"); commodity != "" {
				quadrantDetails += "\n\nCommodity Play: " + cleanHTMLText(commodity)
			}
			
			// Low Impact
			if lowImpact := extractTextBetween(quadrantSection, "Low Impact</h1>", "</p>"); lowImpact != "" {
				quadrantDetails += "\n\nLow Impact: " + cleanHTMLText(lowImpact)
			}
			
			if quadrantDetails != "" && framework.MarketMatrix.Description != "" {
				framework.MarketMatrix.Description += quadrantDetails
			}
		}
	}
	
	// Check if this is the ACP Framework page
	if strings.Contains(html, "ACP Framework Analysis") {
		// Look for scores in the main page summary (if present)
		acpScores := map[string]*int{
			"Audience": &framework.ACPFramework.Audience,
			"Community": &framework.ACPFramework.Community,
			"Product": &framework.ACPFramework.Product,
		}
		
		for name, scorePtr := range acpScores {
			// Try multiple patterns
			patterns := []string{
				fmt.Sprintf(`<span[^>]*>%s</span>.*?<span[^>]*>(\d+)<!-- -->/10</span>`, name),
				fmt.Sprintf(`%s</span>.*?(\d+)<!-- -->/10`, name),
				fmt.Sprintf(`%s.*?(\d+)/10`, name),
			}
			
			for _, pattern := range patterns {
				re := regexp.MustCompile(pattern)
				if matches := re.FindStringSubmatch(html); len(matches) > 1 {
					if score, err := strconv.Atoi(matches[1]); err == nil {
						*scorePtr = score
						break
					}
				}
			}
		}
		
		// Calculate overall ACP score
		if framework.ACPFramework.Audience > 0 && framework.ACPFramework.Community > 0 && framework.ACPFramework.Product > 0 {
			framework.ACPFramework.Overall = (framework.ACPFramework.Audience + framework.ACPFramework.Community + framework.ACPFramework.Product) / 3
		}
	}
	
	// Check if this is the Value Ladder page
	if strings.Contains(html, "Value Ladder Strategy") {
		type LadderStage struct {
			Name string
			Title string
			Price string
			Description string
			ValueProvided string
			Goal string
		}
		
		stages := []LadderStage{}
		
		// Define the stage sections to extract
		stageNames := []string{"LEAD MAGNET", "FRONTEND OFFER", "CORE OFFER", "CONTINUITY PROGRAM", "BACKEND OFFER"}
		
		for _, stageName := range stageNames {
			stage := LadderStage{Name: stageName}
			
			// Extract the section for this stage
			var sectionEnd string
			if stageName == "BACKEND OFFER" {
				sectionEnd = `</div></div></div>`
			} else {
				// Find the next stage name
				for i, name := range stageNames {
					if name == stageName && i < len(stageNames)-1 {
						sectionEnd = stageNames[i+1]
						break
					}
				}
			}
			
			stageSection := extractTextBetween(html, stageName, sectionEnd)
			if stageSection == "" {
				continue
			}
			
			// Extract title
			titleRe := regexp.MustCompile(`<h1[^>]*>([^<]+)</h1>`)
			if matches := titleRe.FindStringSubmatch(stageSection); len(matches) > 1 {
				stage.Title = cleanHTMLText(matches[1])
			}
			
			// Extract price
			priceRe := regexp.MustCompile(`<span[^>]*bg-blue-50[^>]*>([^<]+)</span>`)
			if matches := priceRe.FindStringSubmatch(stageSection); len(matches) > 1 {
				stage.Price = cleanHTMLText(matches[1])
			}
			
			// Extract description (first paragraph after title)
			descRe := regexp.MustCompile(`</h1>.*?<p[^>]*text-gray-600[^>]*>([^<]+)</p>`)
			if matches := descRe.FindStringSubmatch(stageSection); len(matches) > 1 {
				stage.Description = cleanHTMLText(matches[1])
			}
			
			// Extract Value Provided
			valueRe := regexp.MustCompile(`Value Provided</p>.*?<p[^>]*>([^<]+)</p>`)
			if matches := valueRe.FindStringSubmatch(stageSection); len(matches) > 1 {
				stage.ValueProvided = cleanHTMLText(matches[1])
			}
			
			// Extract Goal
			goalRe := regexp.MustCompile(`Goal</p>.*?<p[^>]*>([^<]+)</p>`)
			if matches := goalRe.FindStringSubmatch(stageSection); len(matches) > 1 {
				stage.Goal = cleanHTMLText(matches[1])
			}
			
			stages = append(stages, stage)
		}
		
		// Build the value ladder stages array with detailed info
		ladderStages := []string{}
		for _, stage := range stages {
			// Format: "Stage Name: Title (Price)"
			stageStr := ""
			switch stage.Name {
			case "LEAD MAGNET":
				stageStr = "Lead Magnet"
			case "FRONTEND OFFER":
				stageStr = "Frontend"
			case "CORE OFFER":
				stageStr = "Core"
			case "CONTINUITY PROGRAM":
				stageStr = "Continuity"
			case "BACKEND OFFER":
				stageStr = "Backend"
			}
			
			if stage.Price != "" {
				ladderStages = append(ladderStages, fmt.Sprintf("%s: %s (%s)", stageStr, stage.Title, stage.Price))
			} else {
				ladderStages = append(ladderStages, fmt.Sprintf("%s: %s", stageStr, stage.Title))
			}
			
			// Add detailed information to the description (could be stored separately if needed)
			if stage.Description != "" {
				ladderStages = append(ladderStages, fmt.Sprintf("  - Description: %s", stage.Description))
			}
			if stage.ValueProvided != "" {
				ladderStages = append(ladderStages, fmt.Sprintf("  - Value: %s", stage.ValueProvided))
			}
			if stage.Goal != "" {
				ladderStages = append(ladderStages, fmt.Sprintf("  - Goal: %s", stage.Goal))
			}
		}
		
		if len(ladderStages) > 0 {
			framework.ValueLadderStages = ladderStages
		}
	}
	
	return framework
}

// extractPageData extracts key-value data from a page
func extractPageData(html string) map[string]string {
	data := make(map[string]string)
	
	// Look for key-value patterns in the HTML
	// This is a simplified extraction - adjust based on actual structure
	kvPatterns := []string{
		`<h3[^>]*>([^<]+)</h3>\s*<[^>]+>([^<]+)`,
		`<span[^>]*font-medium[^>]*>([^<]+)</span>\s*<span[^>]*>([^<]+)`,
	}
	
	for _, pattern := range kvPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(html, -1)
		
		for _, match := range matches {
			if len(match) > 2 {
				key := cleanHTMLText(match[1])
				value := cleanHTMLText(match[2])
				if key != "" && value != "" && len(key) < 50 {
					data[key] = value
				}
			}
		}
	}
	
	return data
}

// parseAndSaveData parses all scraped HTML files and saves to JSON
func parseAndSaveData(slug string, pages map[string]string, outputDir string) error {
	idea := &IdeaData{
		Slug: slug,
	}
	
	// Parse main page
	if mainPage, ok := pages["/idea-of-the-day"]; ok {
		idea.Title, idea.Description, idea.Date = extractIdeaInfo(mainPage)
		idea.Tags = extractTags(mainPage)
	}
	
	// Initialize Framework Fit data
	idea.FrameworkFit = &FrameworkData{}
	
	// Parse framework pages
	if valueEqPage, ok := pages["value-equation"]; ok {
		tempFramework := extractFrameworkData(valueEqPage)
		if tempFramework != nil {
			idea.FrameworkFit.ValueEquation = tempFramework.ValueEquation
		}
	}
	
	if matrixPage, ok := pages["value-matrix"]; ok {
		tempFramework := extractFrameworkData(matrixPage)
		if tempFramework != nil {
			idea.FrameworkFit.MarketMatrix = tempFramework.MarketMatrix
		}
	}
	
	if acpPage, ok := pages["acp"]; ok {
		// Extract both ACP detailed data and framework scores
		idea.ACP = extractACPData(acpPage)
		tempFramework := extractFrameworkData(acpPage)
		if tempFramework != nil {
			idea.FrameworkFit.ACPFramework = tempFramework.ACPFramework
		}
	}
	
	if ladderPage, ok := pages["value-ladder"]; ok {
		tempFramework := extractFrameworkData(ladderPage)
		if tempFramework != nil && len(tempFramework.ValueLadderStages) > 0 {
			idea.FrameworkFit.ValueLadderStages = tempFramework.ValueLadderStages
		}
		// Also store as separate page data
		idea.ValueLadder = extractPageData(ladderPage)
	}
	
	// Parse other pages
	pageMapping := map[string]*map[string]string{
		"build/landing-page": &idea.BuildInfo,
		"founder-fit":        &idea.FounderFit,
		"why-now":           &idea.WhyNow,
		"proof-signals":     &idea.ProofSignals,
		"market-gap":        &idea.MarketGap,
		"execution-plan":    &idea.ExecutionPlan,
	}
	
	for pageName, dataPtr := range pageMapping {
		if pageHTML, ok := pages[pageName]; ok {
			*dataPtr = extractPageData(pageHTML)
		}
	}
	
	// Save to JSON file
	jsonData, err := json.MarshalIndent(idea, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %v", err)
	}
	
	filename := fmt.Sprintf("idea_%s_%s.json", slug, time.Now().Format("2006-01-02"))
	filePath := filepath.Join(outputDir, filename)
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %v", err)
	}
	
	log.Printf("✓ Saved idea data to: %s\n", filePath)
	return nil
}
