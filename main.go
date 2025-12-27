package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
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

	// Use mobile user agent to avoid detection
	u := launcher.New().Bin(path).
		Headless(false).
		Set("headless", "new").
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-infobars").
		Set("disable-extensions").
		Set("window-size", "414,896").
		Set("user-agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1").
		Set("lang", "ru-RU,ru").
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

func randomDelay(min, max int) {
	delay := time.Duration(min+rand.Intn(max-min)) * time.Millisecond
	time.Sleep(delay)
}

func (p *OzonParser) createStealthPage(url string) *rod.Page {
	page := stealth.MustPage(p.browser)

	// Set mobile viewport
	page.MustSetViewport(414, 896, 2, true)

	// Add extra evasions
	page.MustEvalOnNewDocument(`
		// Overwrite the 'webdriver' property
		Object.defineProperty(navigator, 'webdriver', {
			get: () => undefined
		});

		// Overwrite the 'plugins' property
		Object.defineProperty(navigator, 'plugins', {
			get: () => [1, 2, 3, 4, 5]
		});

		// Overwrite the 'languages' property
		Object.defineProperty(navigator, 'languages', {
			get: () => ['ru-RU', 'ru', 'en-US', 'en']
		});

		// Mock permissions
		const originalQuery = window.navigator.permissions.query;
		window.navigator.permissions.query = (parameters) => (
			parameters.name === 'notifications' ?
				Promise.resolve({ state: Notification.permission }) :
				originalQuery(parameters)
		);

		// Randomize canvas fingerprint slightly
		const originalGetContext = HTMLCanvasElement.prototype.getContext;
		HTMLCanvasElement.prototype.getContext = function(type, attributes) {
			const context = originalGetContext.call(this, type, attributes);
			if (type === '2d') {
				const originalFillText = context.fillText;
				context.fillText = function(...args) {
					args[1] += Math.random() * 0.01;
					return originalFillText.apply(this, args);
				};
			}
			return context;
		};
	`)

	// Navigate with random delay
	randomDelay(500, 1500)
	page.MustNavigate(url)
	page.MustWaitLoad()

	return page
}

func (p *OzonParser) simulateHuman(page *rod.Page) {
	// Random mouse movements
	for i := 0; i < 3; i++ {
		x := 100 + rand.Intn(1700)
		y := 100 + rand.Intn(800)
		page.Mouse.MustMoveTo(float64(x), float64(y))
		randomDelay(100, 300)
	}

	// Random scroll
	scrollAmount := 200 + rand.Intn(400)
	page.Mouse.MustScroll(0, float64(scrollAmount))
	randomDelay(500, 1000)
}

func (p *OzonParser) Search(query string, maxProducts int) (*SearchResult, error) {
	// Try mobile version first - often has less protection
	url := fmt.Sprintf("https://m.ozon.ru/search/?text=%s&from_global=true", query)

	if p.debug {
		log.Println("Opening:", url)
	}

	page := p.createStealthPage(url)
	defer page.MustClose()

	if p.debug {
		log.Println("Page loaded, simulating human behavior...")
	}

	// Simulate human behavior
	p.simulateHuman(page)

	// Wait for content
	time.Sleep(3 * time.Second)

	html := page.MustHTML()

	// Check for antibot/access restricted
	if strings.Contains(html, "Доступ ограничен") || strings.Contains(html, "Antibot") {
		if p.debug {
			log.Println("Access restricted detected, trying to solve...")
		}

		// More human-like behavior
		p.simulateHuman(page)
		p.simulateHuman(page)

		// Try clicking reload button
		if btn, err := page.Element("#reload-button"); err == nil {
			randomDelay(1000, 2000)
			btn.MustClick()
			time.Sleep(5 * time.Second)
			p.simulateHuman(page)
		}

		time.Sleep(10 * time.Second)
		html = page.MustHTML()

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

	// Scroll to load more products like a human
	for i := 0; i < 5; i++ {
		scrollAmount := 300 + rand.Intn(400)
		page.Mouse.MustScroll(0, float64(scrollAmount))
		randomDelay(800, 1500)
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

		href, err := elem.Attribute("href")
		if err != nil || href == nil {
			continue
		}

		link := *href
		if !strings.HasPrefix(link, "http") {
			link = "https://www.ozon.ru" + link
		}

		if !strings.Contains(link, "/product/") {
			continue
		}

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

		// Get text content
		text, _ := elem.Text()
		lines := strings.Split(strings.TrimSpace(text), "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if len(line) > 10 && !strings.Contains(line, "₽") && product.Name == "" {
				product.Name = line
			}
			if strings.Contains(line, "₽") && product.Price == "" {
				product.Price = line
			}
		}

		if imgEl, err := elem.Element("img"); err == nil {
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

	p.simulateHuman(page)
	time.Sleep(3 * time.Second)

	html := page.MustHTML()

	if strings.Contains(html, "Доступ ограничен") {
		return nil, fmt.Errorf("access restricted by Ozon")
	}

	product := &Product{Link: url}

	if titleEl, err := page.Element("h1"); err == nil {
		if text, err := titleEl.Text(); err == nil {
			product.Name = strings.TrimSpace(text)
		}
	}

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

	if imgEl, err := page.Element("div[data-widget='webGallery'] img"); err == nil {
		if src, err := imgEl.Attribute("src"); err == nil && src != nil {
			product.Image = *src
		}
	}

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

	p.simulateHuman(page)
	time.Sleep(3 * time.Second)

	screenshot, err := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatPng,
		Quality: nil,
	})

	return screenshot, err
}

func main() {
	rand.Seed(time.Now().UnixNano())

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
