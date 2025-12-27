package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

type Product struct {
	Name     string `json:"name"`
	Price    string `json:"price"`
	OldPrice string `json:"old_price,omitempty"`
	Link     string `json:"link"`
	Image    string `json:"image,omitempty"`
	Rating   string `json:"rating,omitempty"`
	Reviews  string `json:"reviews,omitempty"`
	Delivery string `json:"delivery,omitempty"`
}

type SearchResult struct {
	Query    string    `json:"query"`
	Count    int       `json:"count"`
	Products []Product `json:"products"`
}

type OzonParser struct {
	browser *rod.Browser
	debug   bool
}

func NewOzonParser(debug bool) (*OzonParser, error) {
	path, _ := launcher.LookPath()
	if path == "" {
		log.Println("Browser not found, downloading...")
		path = launcher.NewBrowser().MustGet()
	}

	u := launcher.New().Bin(path).
		Headless(true).
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-infobars").
		Set("window-size", "1920,1080").
		MustLaunch()

	browser := rod.New().ControlURL(u).MustConnect()

	return &OzonParser{
		browser: browser,
		debug:   debug,
	}, nil
}

func (p *OzonParser) Close() {
	if p.browser != nil {
		p.browser.MustClose()
	}
}

func (p *OzonParser) createStealthPage(url string) *rod.Page {
	page := stealth.MustPage(p.browser)

	// Set realistic viewport
	page.MustSetViewport(1920, 1080, 1, false)

	// Navigate
	page.MustNavigate(url)
	page.MustWaitLoad()

	return page
}

func (p *OzonParser) Search(query string, maxProducts int) (*SearchResult, error) {
	url := fmt.Sprintf("https://www.ozon.ru/search/?text=%s&from_global=true", query)

	if p.debug {
		log.Println("Opening:", url)
	}

	page := p.createStealthPage(url)
	defer page.MustClose()

	if p.debug {
		log.Println("Page loaded, waiting for content...")
	}

	// Initial wait for page to render
	time.Sleep(5 * time.Second)

	html := page.MustHTML()

	// Check for antibot/access restricted
	if strings.Contains(html, "Доступ ограничен") || strings.Contains(html, "Antibot") {
		if p.debug {
			log.Println("Access restricted detected, waiting and retrying...")
		}

		// Try clicking reload button if exists
		if btn, err := page.Element("#reload-button"); err == nil {
			btn.MustClick()
			time.Sleep(5 * time.Second)
		}

		// Wait longer
		time.Sleep(10 * time.Second)
		html = page.MustHTML()

		// Still blocked?
		if strings.Contains(html, "Доступ ограничен") {
			if p.debug {
				log.Println("Still blocked after retry")
				os.WriteFile("/tmp/ozon_debug.html", []byte(html), 0644)
			}
			return &SearchResult{Query: query, Products: []Product{}}, fmt.Errorf("access restricted by Ozon")
		}
	}

	if p.debug {
		log.Println("Page length:", len(html))
	}

	// Scroll to load more products
	for i := 0; i < 3; i++ {
		page.Mouse.Scroll(0, 500, 1)
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(2 * time.Second)

	result := &SearchResult{
		Query:    query,
		Products: []Product{},
	}

	// Find all product links
	products, _ := page.Elements("a[href*='/product/']")

	if p.debug {
		log.Printf("Found %d product links", len(products))
	}

	if len(products) == 0 {
		if p.debug {
			os.WriteFile("/tmp/ozon_debug.html", []byte(html), 0644)
			log.Println("Debug HTML saved to /tmp/ozon_debug.html")
		}
		return result, nil
	}

	seen := make(map[string]bool)
	count := 0

	for _, elem := range products {
		if count >= maxProducts {
			break
		}

		product := Product{}

		// Get link
		href, err := elem.Attribute("href")
		if err != nil || href == nil {
			continue
		}

		link := *href
		if !strings.HasPrefix(link, "http") {
			link = "https://www.ozon.ru" + link
		}

		// Skip non-product links and duplicates
		if !strings.Contains(link, "/product/") {
			continue
		}

		// Extract product ID from URL to avoid duplicates
		parts := strings.Split(link, "/product/")
		if len(parts) < 2 {
			continue
		}
		productPath := strings.Split(parts[1], "?")[0]
		productPath = strings.Split(productPath, "/")[0]

		if seen[productPath] {
			continue
		}
		seen[productPath] = true

		product.Link = link

		// Try to get parent card element for more info
		parent := elem

		// Get text content - try to find name
		text, _ := parent.Text()
		lines := strings.Split(strings.TrimSpace(text), "\n")

		// First meaningful line is usually the name
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if len(line) > 10 && !strings.Contains(line, "₽") && product.Name == "" {
				product.Name = line
			}
			// Look for price
			if strings.Contains(line, "₽") && product.Price == "" {
				product.Price = line
			}
		}

		// Get image
		if imgEl, err := parent.Element("img"); err == nil {
			if src, err := imgEl.Attribute("src"); err == nil && src != nil {
				product.Image = *src
			}
		}

		if product.Name != "" || product.Link != "" {
			result.Products = append(result.Products, product)
			count++
		}
	}

	result.Count = len(result.Products)
	return result, nil
}

func (p *OzonParser) GetProduct(url string) (*Product, error) {
	if p.debug {
		log.Println("Opening product:", url)
	}

	page := p.createStealthPage(url)
	defer page.MustClose()

	time.Sleep(5 * time.Second)

	html := page.MustHTML()

	if strings.Contains(html, "Доступ ограничен") {
		return nil, fmt.Errorf("access restricted by Ozon")
	}

	product := &Product{Link: url}

	// Get title
	if titleEl, err := page.Element("h1"); err == nil {
		if text, err := titleEl.Text(); err == nil {
			product.Name = strings.TrimSpace(text)
		}
	}

	// Get price
	priceSelectors := []string{
		"div[data-widget='webPrice'] span",
		"span[class*='price']",
	}
	for _, sel := range priceSelectors {
		if priceEl, err := page.Element(sel); err == nil {
			if text, err := priceEl.Text(); err == nil && text != "" && strings.Contains(text, "₽") {
				product.Price = strings.TrimSpace(text)
				break
			}
		}
	}

	// Get main image
	if imgEl, err := page.Element("div[data-widget='webGallery'] img"); err == nil {
		if src, err := imgEl.Attribute("src"); err == nil && src != nil {
			product.Image = *src
		}
	}

	// Get rating
	if ratingEl, err := page.Element("div[data-widget='webReviewProductScore']"); err == nil {
		if text, err := ratingEl.Text(); err == nil {
			product.Rating = strings.TrimSpace(text)
		}
	}

	return product, nil
}

func (p *OzonParser) GetScreenshot(url string) ([]byte, error) {
	page := p.createStealthPage(url)
	defer page.MustClose()

	time.Sleep(5 * time.Second)

	screenshot, err := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatPng,
		Quality: nil,
	})

	return screenshot, err
}

func main() {
	query := "iphone 15"
	if len(os.Args) > 1 {
		query = os.Args[1]
	}

	log.Println("Starting Ozon Parser...")

	parser, err := NewOzonParser(true)
	if err != nil {
		log.Fatal("Failed to create parser:", err)
	}
	defer parser.Close()

	log.Println("Searching for:", query)
	result, err := parser.Search(query, 10)
	if err != nil {
		log.Println("Search error:", err)
	}

	jsonData, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(jsonData))
}
