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
)

type Product struct {
	Name     string   `json:"name"`
	Price    string   `json:"price"`
	OldPrice string   `json:"old_price,omitempty"`
	Link     string   `json:"link"`
	Image    string   `json:"image,omitempty"`
	Rating   string   `json:"rating,omitempty"`
	Reviews  string   `json:"reviews,omitempty"`
	Delivery string   `json:"delivery,omitempty"`
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
	// Find or download browser
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
		Set("disable-web-security").
		Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36").
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

func (p *OzonParser) Search(query string, maxProducts int) (*SearchResult, error) {
	url := fmt.Sprintf("https://www.ozon.ru/search/?text=%s&from_global=true", query)

	if p.debug {
		log.Println("Opening:", url)
	}

	page := p.browser.MustPage(url)
	defer page.MustClose()

	// Set viewport
	page.MustSetViewport(1920, 1080, 1, false)

	// Wait for page load
	page.MustWaitLoad()

	if p.debug {
		log.Println("Page loaded, waiting for content...")
	}

	// Wait a bit for JS to render
	time.Sleep(3 * time.Second)

	html := page.MustHTML()

	// Check for antibot
	if strings.Contains(html, "Antibot") || strings.Contains(html, "antibot") {
		if p.debug {
			log.Println("Antibot detected, waiting...")
		}
		time.Sleep(10 * time.Second)
		html = page.MustHTML()
	}

	if p.debug {
		log.Println("Page length:", len(html))
	}

	// Try to scroll to load more products
	page.Mouse.Scroll(0, 1000, 1)
	time.Sleep(2 * time.Second)

	result := &SearchResult{
		Query:    query,
		Products: []Product{},
	}

	// Try different selectors for product cards
	selectors := []string{
		"div[data-widget='searchResultsV2'] > div > div",
		"div.widget-search-result-container > div > div",
		"div.j8t > div",
		"a[href*='/product/']",
	}

	var products rod.Elements
	for _, sel := range selectors {
		products, _ = page.Elements(sel)
		if len(products) > 0 {
			if p.debug {
				log.Printf("Found %d elements with selector: %s", len(products), sel)
			}
			break
		}
	}

	if len(products) == 0 {
		// Try to extract from page JSON
		if p.debug {
			log.Println("No products found via selectors, trying JSON extraction...")
			// Save HTML for debugging
			os.WriteFile("/tmp/ozon_debug.html", []byte(html), 0644)
			log.Println("Debug HTML saved to /tmp/ozon_debug.html")
		}
		return result, nil
	}

	count := 0
	for _, elem := range products {
		if count >= maxProducts {
			break
		}

		product := Product{}

		// Get link
		if link, err := elem.Attribute("href"); err == nil && link != nil {
			product.Link = *link
			if !strings.HasPrefix(product.Link, "http") {
				product.Link = "https://www.ozon.ru" + product.Link
			}
		} else {
			// Try to find link inside element
			if linkEl, err := elem.Element("a[href*='/product/']"); err == nil {
				if href, err := linkEl.Attribute("href"); err == nil && href != nil {
					product.Link = "https://www.ozon.ru" + *href
				}
			}
		}

		// Skip if no link (not a product card)
		if product.Link == "" || !strings.Contains(product.Link, "/product/") {
			continue
		}

		// Get name
		nameSelectors := []string{"span.tsBody500Medium", "span[class*='tsBody']", "span.tile-name"}
		for _, sel := range nameSelectors {
			if nameEl, err := elem.Element(sel); err == nil {
				if text, err := nameEl.Text(); err == nil && text != "" {
					product.Name = strings.TrimSpace(text)
					break
				}
			}
		}

		// Get price
		priceSelectors := []string{"span.tsHeadline500Medium", "span[class*='price']", "span.c3"}
		for _, sel := range priceSelectors {
			if priceEl, err := elem.Element(sel); err == nil {
				if text, err := priceEl.Text(); err == nil && text != "" {
					product.Price = strings.TrimSpace(text)
					break
				}
			}
		}

		// Get image
		if imgEl, err := elem.Element("img"); err == nil {
			if src, err := imgEl.Attribute("src"); err == nil && src != nil {
				product.Image = *src
			}
		}

		// Get rating
		if ratingEl, err := elem.Element("span[class*='rating'], div[class*='star']"); err == nil {
			if text, err := ratingEl.Text(); err == nil {
				product.Rating = strings.TrimSpace(text)
			}
		}

		if product.Name != "" || product.Price != "" {
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

	page := p.browser.MustPage(url)
	defer page.MustClose()

	page.MustSetViewport(1920, 1080, 1, false)
	page.MustWaitLoad()
	time.Sleep(3 * time.Second)

	html := page.MustHTML()

	// Check for antibot
	if strings.Contains(html, "Antibot") {
		time.Sleep(10 * time.Second)
		html = page.MustHTML()
	}

	product := &Product{Link: url}

	// Get title
	if titleEl, err := page.Element("h1"); err == nil {
		if text, err := titleEl.Text(); err == nil {
			product.Name = strings.TrimSpace(text)
		}
	}

	// Get price - try multiple selectors
	priceSelectors := []string{
		"span[data-widget='webPrice'] span",
		"div[data-widget='webPrice'] span",
		"span.price-number",
	}
	for _, sel := range priceSelectors {
		if priceEl, err := page.Element(sel); err == nil {
			if text, err := priceEl.Text(); err == nil && text != "" {
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
	if ratingEl, err := page.Element("div[data-widget='webReviewRating']"); err == nil {
		if text, err := ratingEl.Text(); err == nil {
			product.Rating = strings.TrimSpace(text)
		}
	}

	return product, nil
}

func (p *OzonParser) GetScreenshot(url string) ([]byte, error) {
	page := p.browser.MustPage(url)
	defer page.MustClose()

	page.MustSetViewport(1920, 1080, 1, false)
	page.MustWaitLoad()
	time.Sleep(3 * time.Second)

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
		log.Fatal("Search failed:", err)
	}

	jsonData, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(jsonData))
}
